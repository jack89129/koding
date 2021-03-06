module.exports = parseTerraformOutput = (response) ->

  # An example of a valid stack template
  # ------------------------------------
  # title: "Default stack",
  # description: "Koding's default stack template for new users",
  # machines: [
  #   {
  #     "label" : "koding-vm-0",
  #     "provider" : "koding",
  #     "instanceType" : "t2.nano",
  #     "provisioners" : [
  #         "devrim/koding-base"
  #     ],
  #     "region" : "us-east-1",
  #     "source_ami" : "ami-a6926dce"
  #   }
  # ],

  out = { machines: [] }

  { machines } = response

  for machine, index in machines

    { label, provider, region, hostQueryString } = machine

    if provider is 'vagrant'
      out.machines.push {
        label, provider
        hostQueryString
        provisioners : []
      }
    else
      { instance_type, ami } = machine.attributes

      out.machines.push {
        label, provider, region
        source_ami   : ami
        instanceType : instance_type
        provisioners : [] # TODO what are we going to do with provisioners? ~ GG
      }

  console.info '[parseTerraformOutput]', out.machines

  return out.machines

