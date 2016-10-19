package topicfeed

import (
	"koding/db/mongodb/modelhelper"
	"math/rand"
	"socialapi/config"
	"socialapi/models"
	"socialapi/workers/topicfeed"
	"testing"

	"github.com/koding/runner"

	"github.com/koding/bongo"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMarkedAsTroll(t *testing.T) {
	Convey("while extracting topics", t, func() {
		Convey("duplicates should be returned as unique", func() {
			So(len(extractTopics("hi #topic #topic my topic")), ShouldEqual, 1)
		})

		Convey("public should be removed from topics list", func() {
			topics := extractTopics("hi #topic #public my topic")
			So(len(topics), ShouldEqual, 1)
			So(topics[0], ShouldEqual, "topic")
		})

		Convey("duplicate public should be removed from topics list", func() {
			topics := extractTopics("hi #public  #public  my topic")
			So(len(topics), ShouldEqual, 0)
		})
	})
}

func TestIsEligible(t *testing.T) {
	Convey("while testing isEligible", t, func() {
		Convey("initial channel id should be set", func() {
			c := models.NewChannelMessage()
			c.InitialChannelId = 0
			eligible, err := isEligible(c)
			So(err, ShouldBeNil)
			So(eligible, ShouldBeFalse)
		})

		Convey("type_constant should be Post", func() {
			c := models.NewChannelMessage()
			eligible, err := isEligible(c)
			So(err, ShouldBeNil)
			So(eligible, ShouldBeFalse)

			Convey("when it is set to Post, should be eligible", func() {
				c.InitialChannelId = rand.Int63()
				c.TypeConstant = models.ChannelMessage_TYPE_POST
				eligible, err := isEligible(c)
				So(err, ShouldBeNil)
				So(eligible, ShouldBeTrue)
			})
		})
	})
}

func TestMessageSaved(t *testing.T) {
	r := runner.New("test")
	if err := r.Init(); err != nil {
		t.Fatalf("couldnt start bongo %s", err.Error())
	}
	defer r.Close()

	// init mongo connection
	appConfig := config.MustRead(r.Conf.Path)
	modelhelper.Initialize(appConfig.Mongo)
	defer modelhelper.Close()

	controller := topicfeed.New(r.Log, appConfig)

	Convey("while testing MessageSaved", t, func() {

		Convey("if message posted to a channel directly", func() {
			account, groupChannel, groupName := models.CreateRandomGroupDataWithChecks()

			topicChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, groupName)
			cp, err := topicChannel.AddParticipant(account.Id)
			So(err, ShouldBeNil)
			So(cp, ShouldNotBeNil)

			// just a random topic name
			topicName := topicChannel.Name

			c := models.NewChannelMessage()
			c.InitialChannelId = topicChannel.Id
			c.AccountId = account.Id
			c.Body = "my test post without a hashTag" + topicName
			c.TypeConstant = models.ChannelMessage_TYPE_POST

			// create with unscoped
			err = bongo.B.Unscoped().Table(c.TableName()).Create(c).Error
			So(err, ShouldBeNil)

			So(controller.MessageSaved(c), ShouldBeNil)

			Convey("should be posted to group channel too", func() {
				m, err := groupChannel.FetchLastMessage()
				So(err, ShouldBeNil)
				So(m, ShouldNotBeNil)
				So(m.Id, ShouldEqual, c.Id)
			})

			Convey("unread count of topic channel should be 0", func() {
				cml := models.NewChannelMessageList()
				unreadCount, err := cml.UnreadCount(cp)
				So(err, ShouldBeNil)
				So(unreadCount, ShouldEqual, 0)
			})
		})

		Convey("for non koding groups", func() {
			account, _, groupName := models.CreateRandomGroupDataWithChecks()

			Convey("even if channel exists", func() {
				topicChannel1 := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, groupName)
				topicChannel2 := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, groupName)

				// just a random topic name
				c := models.NewChannelMessage()
				c.InitialChannelId = topicChannel1.Id
				c.AccountId = account.Id
				c.Body = "my test topic #" + topicChannel2.Name
				c.TypeConstant = models.ChannelMessage_TYPE_POST

				// create with unscoped
				err := bongo.B.Unscoped().Table(c.TableName()).Create(c).Error
				So(err, ShouldBeNil)

				So(controller.MessageSaved(c), ShouldBeNil)

				Convey("we should not add the message to them", func() {
					m, err := topicChannel2.FetchLastMessage()
					So(err, ShouldBeNil)
					So(m, ShouldBeNil)
				})
			})

			Convey("if message has two hashTags", func() {
				topicChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, groupName)

				// just a random topic name
				topicName := topicChannel.Name
				c := models.NewChannelMessage()
				c.InitialChannelId = topicChannel.Id
				c.AccountId = account.Id
				c.Body = "my test topic #" + topicName
				c.TypeConstant = models.ChannelMessage_TYPE_POST

				// create with unscoped
				err := bongo.B.Unscoped().Table(c.TableName()).Create(c).Error
				So(err, ShouldBeNil)

				So(controller.MessageSaved(c), ShouldBeNil)

				Convey("we should not add the message to them", func() {
					m, err := topicChannel.FetchLastMessage()
					So(err, ShouldBeNil)
					So(m, ShouldBeNil)
				})
			})
		})
	})
}

func TestFetchTopicChannel(t *testing.T) {
	r := runner.New("test")
	if err := r.Init(); err != nil {
		t.Fatalf("couldnt start bongo %s", err.Error())
	}
	defer r.Close()

	appConfig := config.MustRead(r.Conf.Path)
	controller := New(r.Log, appConfig)

	Convey("while testing fetchTopicChannel", t, func() {
		account := models.CreateAccountWithTest()
		groupChannel := models.CreateTypedPublicChannelWithTest(
			account.Id,
			models.Channel_TYPE_GROUP,
		)

		normalChannel := models.NewChannel()
		normalChannel.CreatorId = account.Id
		normalChannel.GroupName = groupChannel.GroupName
		normalChannel.TypeConstant = models.Channel_TYPE_TOPIC
		normalChannel.PrivacyConstant = models.Channel_PRIVACY_PUBLIC
		So(normalChannel.Create(), ShouldBeNil)

		Convey("unlinked channels should be fetched normally", func() {
			c1, err := controller.fetchTopicChannel(normalChannel.GroupName, normalChannel.Name)
			So(err, ShouldBeNil)
			So(c1, ShouldNotBeNil)
			So(c1.Id, ShouldEqual, normalChannel.Id)
		})
	})
}

func TestMessageUpdated(t *testing.T) {
	r := runner.New("test")
	if err := r.Init(); err != nil {
		t.Fatalf("couldnt start bongo %s", err.Error())
	}
	defer r.Close()

	// init mongo connection
	appConfig := config.MustRead(r.Conf.Path)
	modelhelper.Initialize(appConfig.Mongo)
	defer modelhelper.Close()

	controller := topicfeed.New(r.Log, appConfig)

	Convey("while testing MessageUpdated", t, func() {

		Convey("if message is non-koding post", func() {
			account, groupChannel, groupName := models.CreateRandomGroupDataWithChecks()

			topicChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, groupName)

			topicName := topicChannel.Name
			c := models.NewChannelMessage()
			c.InitialChannelId = groupChannel.Id
			c.AccountId = account.Id
			c.Body = "my test post with a hashTag #" + topicName
			c.TypeConstant = models.ChannelMessage_TYPE_POST

			// create with unscoped
			err := bongo.B.Unscoped().Table(c.TableName()).Create(c).Error
			So(err, ShouldBeNil)

			So(controller.MessageUpdated(c), ShouldBeNil)

			Convey("should not be posted to other channel", func() {
				m, err := topicChannel.FetchLastMessage()
				So(err, ShouldBeNil)
				So(m, ShouldBeNil)
			})
		})
		Convey("if message is a koding post", func() {
			account := models.CreateAccountWithTest()
			groupChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_GROUP, "koding")

			topicChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, "koding")

			topicName := topicChannel.Name
			c := models.NewChannelMessage()
			c.InitialChannelId = groupChannel.Id
			c.AccountId = account.Id
			c.Body = "my test post with a hashTag #" + topicName
			c.TypeConstant = models.ChannelMessage_TYPE_POST

			// create with unscoped
			err := bongo.B.Unscoped().Table(c.TableName()).Create(c).Error
			So(err, ShouldBeNil)

			So(controller.MessageUpdated(c), ShouldBeNil)

			Convey("should be posted to other channel", func() {
				m, err := topicChannel.FetchLastMessage()
				So(err, ShouldBeNil)
				So(m, ShouldNotBeNil)
				So(m.Id, ShouldEqual, c.Id)
			})
		})
	})
}

func TestMessageDeleted(t *testing.T) {
	r := runner.New("test")
	if err := r.Init(); err != nil {
		t.Fatalf("couldnt start bongo %s", err.Error())
	}
	defer r.Close()

	// init mongo connection
	appConfig := config.MustRead(r.Conf.Path)
	modelhelper.Initialize(appConfig.Mongo)
	defer modelhelper.Close()

	controller := topicfeed.New(r.Log, appConfig)

	Convey("while testing MessageDeleted", t, func() {

		account := models.CreateAccountWithTest()
		groupChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_GROUP, "koding")
		topicChannel := models.CreateTypedGroupedChannelWithTest(account.Id, models.Channel_TYPE_TOPIC, "koding")

		topicName := topicChannel.Name
		c := models.NewChannelMessage()
		c.InitialChannelId = groupChannel.Id
		c.AccountId = account.Id
		c.Body = "my test post with a hashTag #" + topicName
		c.TypeConstant = models.ChannelMessage_TYPE_POST

		// create with unscoped
		err := bongo.B.Unscoped().Table(c.TableName()).Create(c).Error
		So(err, ShouldBeNil)

		So(controller.MessageSaved(c), ShouldBeNil)
		So(controller.MessageDeleted(c), ShouldBeNil)

		Convey("messages should be deleted from other channels", func() {
			m, err := topicChannel.FetchLastMessage()
			So(err, ShouldBeNil)
			So(m, ShouldBeNil)
		})
	})
}
