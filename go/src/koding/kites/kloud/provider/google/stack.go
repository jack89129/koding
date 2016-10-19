package google

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"koding/kites/kloud/stack"
	"koding/kites/kloud/stack/provider"
	"koding/kites/kloud/userdata"

	compute "google.golang.org/api/compute/v1"
)

//go:generate $GOPATH/bin/go-bindata -mode 420 -modtime 1475345133 -pkg google -o bootstrap.json.tmpl.go bootstrap.json.tmpl
//go:generate go fmt bootstrap.json.tmpl.go

const (
	defaultMachineType  = "n1-standard-1"   // 1vCPU, 3.75GB memory.
	defaultMachineImage = "ubuntu-1404-lts" // From image family, size: 10GB.
)

var bootstrap = template.Must(template.New("").Parse(mustAsset("bootstrap.json.tmpl")))

type bootstrapConfig struct{}

// Stack implements the stack.Stack interface.
type Stack struct {
	*provider.BaseStack
}

var (
	_ provider.Stack = (*Stack)(nil) // public API
	_ stack.Stacker  = (*Stack)(nil) // internal API
)

func newStack(bs *provider.BaseStack) (provider.Stack, error) {
	return &Stack{BaseStack: bs}, nil
}

func (s *Stack) VerifyCredential(c *stack.Credential) error {
	cred := c.Credential.(*Cred)

	if err := cred.Valid(); err != nil {
		return err
	}

	computeService, err := cred.ComputeService()
	if err != nil {
		return err
	}

	// Try to get project info. If there is no error, project name and JSON key
	// are valid.
	_, err = compute.NewProjectsService(computeService).Get(cred.Project).Do()
	return err
}

func (s *Stack) BootstrapTemplates(c *stack.Credential) ([]*stack.Template, error) {
	t, err := newBootstrapTemplate(&bootstrapConfig{})
	if err != nil {
		return nil, err
	}

	cred := c.Credential.(*Cred)
	t.Key = cred.Project + "-" + s.BootstrapArg().GroupName + "-" + c.Identifier
	return []*stack.Template{t}, nil
}

func (s *Stack) ApplyTemplate(c *stack.Credential) (*stack.Template, error) {
	t := s.Builder.Template

	var resource struct {
		GCInstance map[string]map[string]interface{} `hcl:"google_compute_instance"`
	}

	if err := t.DecodeResource(&resource); err != nil {
		return nil, err
	}

	if len(resource.GCInstance) == 0 {
		return nil, fmt.Errorf("there are no Google compute instances defined")
	}

	for resourceName, instance := range resource.GCInstance {
		// Set default machine type if user didn't define it herself.
		if mt, ok := instance["machine_type"]; !ok {
			instance["machine_type"] = defaultMachineType
		} else {
			if mtstr, ok := mt.(string); ok && mtstr == "" {
				instance["machine_type"] = defaultMachineType
			}
		}

		// Set default zone if user didn't define it herself.
		const lookupZone = "${lookup(var.zones, var.google_region)}"
		if z, ok := instance["zone"]; !ok {
			instance["zone"] = lookupZone
		} else {
			if zstr, ok := z.(string); ok && zstr == "" {
				instance["zone"] = lookupZone
			}
		}

		// Set default image for disk if user didn't define it herself.
		if _, ok := instance["disk"]; !ok {
			instance["disk"] = map[string]interface{}{
				"image": defaultMachineImage,
			}
		}

		// Set default network interface if user didn't define it herself.
		if _, ok := instance["network_interface"]; !ok {
			instance["network_interface"] = map[string]interface{}{
				"network":       "default",
				"access_config": map[string]interface{}{},
			}
		}

		// Instance name is always required.
		instanceName, ok := instance["name"]
		if !ok {
			return nil, fmt.Errorf("%q instance name is required", resourceName)
		}
		if instr, ok := instanceName.(string); !ok || instr == "" {
			return nil, fmt.Errorf("%q instance name is invalid: %v", resourceName, instanceName)
		}

		// means there will be several instances, we need to create a userdata
		// with count interpolation, because each machine must have an unique
		// kite id.
		count := 1
		if n, ok := instance["count"].(int); ok && n > 1 {
			count = n
		}

		labels := []string{resourceName}
		if count > 1 {
			labels = labels[:0]
			for i := 0; i < count; i++ {
				labels = append(labels, fmt.Sprintf("%s.%d", resourceName, i))
			}

			// instance names must be unique, if user set count property, she
			// need to use interpolation in name attribute. If she doesn't, we
			// will attach index number at the end of name string.
			if instr := instanceName.(string); !strings.Contains(instr, "${count.index}") {
				instance["name"] = instr + "-${count.index}"
			}
		}

		kiteKeyName := fmt.Sprintf("kitekeys_%s", resourceName)

		// Cloud-Init can be injected only via "user-data" field defined in
		// root "metadata" object.
		metadata, ok := instance["metadata"].(map[string]interface{})
		if !ok {
			metadata = make(map[string]interface{})
		}
		s.Builder.InterpolateField(metadata, resourceName, "user-data")

		// this part will be the same for all machines
		userCfg := &userdata.CloudInitConfig{
			Username: s.Req.Username,
			Groups:   []string{"sudo"},
			Hostname: s.Req.Username, // no typo here. hostname = username
			KiteKey:  fmt.Sprintf("${lookup(var.%s, count.index)}", kiteKeyName),
		}

		if s, ok := metadata["user-data"].(string); ok {
			userCfg.UserData = s
		}

		userdata, err := s.Session.Userdata.Create(userCfg)
		if err != nil {
			return nil, err
		}

		metadata["user-data"] = string(userdata)
		instance["metadata"] = addPublicKey(metadata, s.Req.Username, s.Keys.PublicKey)

		// create independent kiteKey for each machine and create a Terraform
		// lookup map, which is used in conjuctuon with the `count.index`
		countKeys := make(map[string]string, count)
		for i, label := range labels {
			kiteKey, err := s.BuildKiteKey(label, s.Req.Username)
			if err != nil {
				return nil, err
			}

			countKeys[strconv.Itoa(i)] = kiteKey
		}

		t.Variable[kiteKeyName] = map[string]interface{}{
			"default": countKeys,
		}

		resource.GCInstance[resourceName] = instance
	}

	t.Resource["google_compute_instance"] = resource.GCInstance

	if err := t.Flush(); err != nil {
		return nil, err
	}

	err := t.ShadowVariables("FORBIDDEN", "google_credentials")
	if err != nil {
		return nil, err
	}

	content, err := t.JsonOutput()
	if err != nil {
		return nil, err
	}

	return &stack.Template{
		Content: content,
	}, nil
}

func (s *Stack) BootstrapArg() *stack.BootstrapRequest {
	return s.BaseStack.Arg.(*stack.BootstrapRequest)
}

// addPublicKey injects to user-publicKey ssh pair into instance metadata.
func addPublicKey(metadata map[string]interface{}, user, publicKey string) map[string]interface{} {
	key := "ssh-keys"
	// Fall back to depricated sshKeys instance level value. It may be used by
	// the user who creates instances that don't support newer ssh-keys format.
	if _, ok := metadata["sshKeys"]; ok {
		// User specified both fields. In that case we will use newer format.
		if _, ok := metadata[key]; !ok {
			key = "sshKeys"
		}
	}

	sshKeyNew := user + ":" + publicKey
	if sshKeyCpy, ok := metadata[key].(string); ok && sshKeyCpy != "" {
		sshKeyNew = sshKeyNew + `\n` + sshKeyCpy
	}

	metadata[key] = sshKeyNew
	return metadata
}

func mustAsset(s string) string {
	p, err := Asset(s)
	if err != nil {
		panic(err)
	}
	return string(p)
}

func newBootstrapTemplate(cfg *bootstrapConfig) (*stack.Template, error) {
	var buf bytes.Buffer

	if err := bootstrap.Execute(&buf, cfg); err != nil {
		return nil, err
	}

	return &stack.Template{
		Content: buf.String(),
	}, nil
}
