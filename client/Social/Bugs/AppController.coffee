class BugReportController extends AppController

  KD.registerAppClass this,
    name         : "Bugs"
    route        : "/Bugs"
    version      : "1.0"

  constructor:(options = {}, data)->
    options.view    = new BugReportMainView
      cssClass      : "content-page bugreports"
    options.appInfo =
      name          : 'Bugs'
    super options, data
    @lastTo
    @lastFrom
    @on "LazyLoadThresholdReached", => @feedController?.loadFeed()

  loadView:(mainView)->
    @createFeed mainView

  createFeed: (view)->
    options =
      feedId              : 'apps.bugreport'
      itemClass           : BugStatusItemList
      limitPerPage        : 20
      useHeaderNav        : yes
      filter              :
        all               :
          title           : "Reported Bugs"
          noItemFoundText : "There is no reported bugs"
          dataSource      : (selector, options, callback) =>
            options["tag"]     = "bug"
            options["tagType"] = "user-tag"
            @fetch selector, options, callback
        fixed             :
          title           : "Fixed Bugs"
          noItemFoundText : "There is no fixed bugs"
          dataSource      : (selector, options, callback) =>
            options["tag"]     = "fixed"
            options["tagType"] = "system-tag"
            @fetch selector, options, callback
        changelog         :
          title           : "Change Log"
          noItemFoundText : "There is no changelog"
          dataSource      : (selector, options, callback) =>
            options["tag"]     = "changelog"
            options["tagType"] = "system-tag"
            @fetch selector, options, callback
      sort                :
        'meta.modifiedAt' :
          title           : "Latest Bugs"
          direction       : -1

    KD.getSingleton("appManager").tell 'Feeder', 'createContentFeedController', options, (controller)=>
      view.mainBlock.addSubView controller.getView()
      @feedController = controller
      @feedController.on "FilterChanged", =>
        delete @lastTo
      @getOptions().view.setOptions controller
      @emit 'ready'

  fetch:(selector, options ,callback)->
    {JNewStatusUpdate, JTag} = KD.remote.api
    JTag.one title : options.tag, category : options.tagType, (err, sysTag) =>
      return err if err
      selector  =
        slug    : sysTag.slug
        limit   : options.limit
        to      : @lastTo

      JNewStatusUpdate.fetchTopicFeed selector, (err, activities = []) =>
        @extractMessageTimeStamps activities
        activities?.map (activity) ->
          activity.on "TagsUpdated", (tags) ->
            activity.tags = KD.remote.revive tags
        callback err, activities

  setLastTimestamps:(from, to)->
    if from
      @lastTo   = to
      @lastFrom = from
    else
      @reachedEndOfActivities = yes

  # Store first & last cache activity timestamp.
  extractMessageTimeStamps: (messages)->
    return  if messages.length is 0
    from = new Date(messages.last.meta.createdAt).getTime()
    to   = new Date(messages.first.meta.createdAt).getTime()
    @setLastTimestamps to, from #from, to
