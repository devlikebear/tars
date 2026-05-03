export type Locale = 'en' | 'ko'

export type Translations = {
  common: {
    actions: {
      askAI: string
      refresh: string
      reload: string
      save: string
      cancel: string
      delete: string
      rename: string
      compact: string
      dismiss: string
      close: string
      loading: string
      working: string
      saving: string
    }
    states: {
      connected: string
      disconnected: string
      never: string
      unknown: string
    }
  }
  nav: {
    mainNavigation: string
    navigationSuffix: string
    groups: {
      work: string
      operate: string
      setup: string
    }
    items: {
      chat: string
      lineage: string
      plans: string
      memory: string
      sysprompt: string
      extensions: string
      agentruntime: string
      channels: string
      ops: string
      cron: string
      logs: string
      analytics: string
      pulse: string
      reflection: string
      config: string
    }
  }
  header: {
    title: string
    notifications: string
    markAllRead: string
    filters: {
      all: string
      unread: string
      read: string
    }
    empty: {
      all: string
      unread: string
      read: string
    }
    locale: {
      label: string
      en: string
      ko: string
    }
    budget: {
      label: string
      title: (used: string, budget: string, percent: number) => string
    }
    time: {
      justNow: string
      minutesAgo: (count: number) => string
      hoursAgo: (count: number) => string
    }
  }
  sessions: {
    title: string
    subtitle: string
    showWorkers: string
    searchPlaceholder: string
    filters: {
      all: string
      session: string
      main: string
      worker: string
    }
    sort: {
      recent: string
      name: string
    }
    loading: string
    searchingTranscripts: string
    empty: string
    noMatches: string
    transcriptMatches: string
    autoTitle: string
    confirmDelete: string
    clickAgainToConfirm: string
  }
  memory: {
    title: string
    assetCount: (count: number) => string
    introLabel: string
    introTitle: string
    introBody: string
    introItems: {
      memory: string
      experiences: string
      dailyLogs: string
      semanticIndex: string
    }
    introHeadings: {
      memory: string
      experiences: string
      dailyLogs: string
      semanticIndex: string
    }
    introFooter: string
    stats: {
      selectedAsset: string
      search: string
      hits: (count: number) => string
      inbox: string
      pending: (count: number) => string
    }
    tabs: {
      inbox: string
      storedKnowledge: string
      trySearch: string
    }
    inbox: {
      title: string
      subtitle: string
      loading: string
      empty: string
      approve: string
      reject: string
      merge: string
      reviewing: string
      provenance: string
      similar: string
      conflicts: string
      mergedInto: (target: string) => string
      approved: string
      rejected: string
      merged: string
    }
    assets: string
    editor: string
    selectFile: string
    loadingAssets: string
    emptyAssets: string
    stale: string
    filledBy: string
    readBy: string
    placeholders: {
      editor: string
      query: string
      sessionId: string
    }
    search: {
      title: string
      modeLabel: string
      toolPath: string
      prefetchPath: string
      query: string
      limit: string
      sessionId: string
      runSearch: string
      runPrefetch: string
      running: string
      results: string
      matches: (count: number) => string
      prefetchEmpty: string
      searchEmpty: string
      noPriorMatches: string
      noMatches: string
      priorContext: string
      dailyLogs: string
      sessionHistory: string
      defaultToolMeta: string
    }
  }
  onboarding: {
    kicker: {
      firstRun: string
      reentry: string
    }
    title: string
    subtitleFirstRun: string
    subtitleReentry: string
    steps: {
      provider: string
      tiers: string
      review: string
      restart: string
      saved: string
    }
    errors: {
      inputCheck: string
      saveFailed: string
      restartFailed: string
      refreshFailed: string
    }
    step1: {
      cardTitle: string
      kindLabel: string
      kindHint: string
      kindPlaceholder: string
      aliasLabel: string
      aliasHint: string
      aliasPlaceholder: string
      authModeLabel: string
      authModeHint: string
      apiKeyLabel: string
      apiKeyKeepHint: string
      apiKeyPlaceholderKeep: string
      apiKeyPlaceholderNew: string
      baseUrlLabel: string
      baseUrlHint: string
      hintOauthTitle: string
      hintOauthBody: string
      hintCliTitle: string
      hintCliBody: string
      nextButton: string
      selectProviderLabel: string
      addNewProviderOption: string
    }
    step2: {
      cardTitle: string
      cardMeta: string
      modelsSourceLive: (count: number) => string
      modelsSourceStatic: (date: string, count: number) => string
      modelsSourceStaticEmpty: (date: string) => string
      refreshButton: string
      refreshing: string
      providerAliasLabel: string
      modelLabel: string
      reasoningLabel: string
      reasoningHint: string
      reasoningDefault: string
      backButton: string
      nextButton: string
      refreshErrorEmpty: string
    }
    step3: {
      cardTitle: string
      saveLocation: string
      providerHeading: string
      tiersHeading: string
      aliasField: string
      kindField: string
      authModeField: string
      apiKeyField: string
      baseUrlField: string
      tierField: string
      providerField: string
      modelField: string
      reasoningField: string
      apiKeyKept: string
      none: string
      defaultLabel: string
      backButton: string
      saveOnlyButton: string
      saveAndRestartButton: string
    }
    saved: {
      title: string
      body: string
      laterButton: string
      restartNowButton: string
    }
    restart: {
      patchingTitle: string
      patchingBody: string
      restartingTitle: string
      restartingBody: string
      pollingTitle: string
      pollingBody: string
      readyTitle: string
      readyBody: string
      timeoutTitle: string
      timeoutBody: string
      refreshButton: string
    }
  }
  shell: {
    setupOnlyKicker: string
    setupOnlyBody: string
  }
  chat: {
    pinned: string
  }
  configWizardCard: {
    kicker: string
    body: string
    button: string
  }
  cron: {
    eyebrow: string
    title: string
    refresh: string
    summaryAriaLabel: string
    metricActive: string
    metricPaused: string
    metricDone: string
    metricLoadedRuns: string
    createPanelAriaLabel: string
    newJob: string
    deliveryDailyLog: string
    deliveryMain: string
    deliveryBoth: string
    deliveryNone: string
    deliveryBound: string
    nameLabel: string
    namePlaceholder: string
    scheduleLabel: string
    schedulePlaceholder: string
    deliveryLabel: string
    promptLabel: string
    promptPlaceholder: string
    creating: string
    createButton: string
    jobsAriaLabel: string
    jobsTitle: string
    totalSuffix: (count: number) => string
    loadingJobs: string
    noJobs: string
    bucketActive: string
    bucketPaused: string
    bucketDone: string
    statusFailed: string
    statusDone: string
    statusActive: string
    statusPaused: string
    untitled: string
    nextCompleted: string
    nextPaused: string
    nextSchedule: string
    nextAt: (time: string) => string
    nextAfter: (relative: string) => string
    nextTick: string
    pause: string
    resume: string
    runNow: string
    running: string
    confirm: string
    delete: string
    runHistory: string
    lastRun: (relative: string) => string
    loadingRuns: string
    noRuns: string
    runOk: string
    runError: string
    never: string
    secondsAgo: (n: number) => string
    minutesAgo: (n: number) => string
    hoursAgo: (n: number) => string
    daysAgo: (n: number) => string
  }
  logs: {
    kicker: string
    title: string
    autoToggle: string
    refresh: string
    refreshing: string
    file: string
    runtimeLog: string
    level: string
    levelOptions: {
      all: string
      debug: string
      info: string
      warn: string
      error: string
    }
    component: string
    componentPlaceholder: string
    lines: string
    streamTitle: string
    linesSuffix: (count: number) => string
    fileMissing: string
    loadingLogs: string
    noLines: string
    rawSummary: string
    filesTitle: string
  }
  analytics: {
    kicker: string
    title: string
    periodAriaLabel: string
    periodSuffix: string
    summary: {
      totalTokens: string
      tokensInOut: (input: string, output: string) => string
      sessions: string
      callsSuffix: (calls: string) => string
      avgPerSession: string
      tokensSuffix: string
      estimatedCost: string
      daysSuffix: (days: number) => string
    }
    chart: {
      title: string
      legendInput: string
      legendOutput: string
      ariaLabel: string
      loading: string
      emptyTitle: string
      emptyBody: string
      barTitle: (day: string, input: string, output: string) => string
    }
    models: {
      title: string
      empty: string
      thModel: string
      thSessions: string
      thInput: string
      thOutput: string
      thCost: string
    }
    skills: {
      title: string
      empty: string
      callsSuffix: (count: string) => string
    }
  }
  tasks: {
    title: string
    loading: string
    empty: string
    emptyHint: string
    planReady: string
    planReadyHint: (count: number) => string
    approveRun: string
    editPlan: string
    discard: string
    plan: string
    pause: string
    resume: string
    abort: string
    taskTitle: string
    descriptionOptional: string
    removeTask: string
    addTask: string
    saveChanges: string
    skipTask: string
    archive: {
      pastPlans: (count: number) => string
      loading: string
      empty: string
      archivedAt: string
      createdAt: (value: string) => string
    }
    stats: {
      active: string
      pending: string
      done: string
      skipped: string
    }
    confirm: {
      discard: string
      abort: string
    }
  }
  ops: {
    title: string
    subtitle: string
    refresh: string
    loading: string
    approvalsCard: string
    pendingBadge: (count: number) => string
    creating: string
    newCleanupPlan: string
    cleanupPlanTooltip: string
    emptyKicker: string
    emptyTitle: string
    emptyBody: string
    triggersLabel: string
    stepsLabel: string
    triggers: {
      cleanupTitle: string
      cleanupDetail: string
      cleanupState: string
      pulseTitle: string
      pulseDetail: string
      pulseState: string
    }
    steps: {
      reviewTitle: string
      reviewDetail: string
      chooseTitle: string
      chooseDetail: string
      resultTitle: string
      resultDetail: string
    }
    gitDestructive: string
    gitAction: string
    candidatesSuffix: (count: number) => string
    approve: string
    reject: string
    auditTitle: string
    auditEventsSuffix: (count: number) => string
    refreshing: string
    noEvents: string
    destructiveBadge: string
    errorReview: string
    errorAudit: string
    errorCreate: string
  }
  reflection: {
    loading: string
    introTitle: string
    systemSurfaceBadge: string
    introLead: (window: string) => string
    jobs: {
      memoryLabel: string
      memoryDesc: (hours: string) => string
      cleanupLabel: string
      cleanupDesc: (age: string) => string
    }
    introAction1: string
    introAction2: string
    introAction3: string
    statusTitle: string
    enabled: string
    disabled: string
    consecutiveFailures: (count: number) => string
    facts: {
      sleepWindow: string
      timezone: string
      tickInterval: string
      emptySessionAge: string
      memoryLookback: string
      lastRun: string
      lastSuccess: string
      totalRuns: string
      successes: string
      failures: string
    }
    lastRunTitle: string
    successBadge: string
    failedBadge: string
    startedLabel: string
    finishedLabel: string
    jobOk: string
    jobFail: string
    jobChanged: string
    noRunsYet: string
    expectedOutput: string
    previewMemory: string
    previewCleanup: string
    previewFailure: string
    runNowButton: string
    runningButton: string
    disabledHint: string
    bypassHint: string
    runResultTitle: string
    durationLabel: string
    runTotalsTitle: string
    metrics: {
      experiencesLabel: string
      removedLabel: string
      compiledLabel: string
      notReported: string
      firstRun: string
      noPreviousValue: string
      sameAsLastRun: string
    }
    jobDetails: {
      sessionsScanned: (count: number) => string
      turnsProcessed: (count: number) => string
      experiencesExtracted: (count: number) => string
      kbEntriesCompiled: (count: number) => string
      emptySessionsRemoved: (count: number) => string
      skipped: (count: number) => string
    }
    relativeTime: {
      never: string
      secondsAgo: (n: number) => string
      minutesAgo: (n: number) => string
      hoursAgo: (n: number) => string
      daysAgo: (n: number) => string
    }
    recentRunsTitle: string
    jobsCount: (count: number) => string
    errorLoadStatus: string
    errorRunFailed: string
  }
  pulse: {
    loading: string
    kicker: string
    title: string
    introBody: (interval: string) => string
    policySource: string
    watchTargets: string
    whenSignalsAppear: string
    whenSignalsBody: string
    watchItems: {
      cronFailures: { label: string; detail: string }
      stuckRuns: { label: string; detail: string }
      diskPressure: { label: string; detail: string }
      telegramFailures: { label: string; detail: string }
      reflectionFailures: { label: string; detail: string }
    }
    decisions: {
      ignore: { action: string; detail: string }
      notify: { action: string; detail: string }
      autofix: { action: string; detail: string }
    }
    statusTitle: string
    enabled: string
    disabled: string
    facts: {
      interval: string
      activeHours: string
      timezone: string
      minSeverity: string
      minSeverityNote: string
      lastTick: string
      totalTicks: string
      decisions: string
      notifies: string
      autofixes: string
    }
    severityGuideTitle: string
    severityGuideNote: string
    severityGuide: {
      cronFailures: { label: string; info: string }
      diskPressure: { label: string; info: string }
      stuckRun: { label: string; info: string }
      telegramDelivery: { label: string; info: string }
      reflectionHealth: { label: string; info: string }
    }
    severityWarn: {
      cronFailures: (threshold: number) => string
      diskPressure: (percent: string) => string
      stuckRun: (minutes: number) => string
      telegramDelivery: string
      reflectionHealth: string
    }
    severityError: {
      cronFailures: (threshold: number) => string
      diskPressure: (percent: string) => string
      stuckRun: string
      telegramDelivery: string
      reflectionHealth: string
    }
    lastSeenTitle: string
    lastDecisionTitle: string
    noDecisionsYet: string
    runTickNow: string
    running: string
    disabledHint: string
    tickResultTitle: string
    tickBadge: {
      skipped: string
      deciderRan: string
      notified: string
      autofixOk: string
      error: string
      noSignals: string
      signalCount: (count: number) => string
    }
    recentTicksTitle: string
    recentSummary: {
      lastTicks: (count: number) => string
      allClear: string
      signalTicks: (count: number, allClear: number) => string
      warnings: (count: number) => string
      errors: (count: number) => string
      autofixes: (count: number) => string
    }
    incidentCardsTitle: string
    likelyCause: string
    evidence: string
    recommendedAction: string
    openAffectedPage: string
    recheck: string
    signalTicksTitle: string
    relativeTime: {
      never: string
      secondsAgo: (n: number) => string
      minutesAgo: (n: number) => string
      hoursAgo: (n: number) => string
      daysAgo: (n: number) => string
    }
    errorLoadStatus: string
    errorRunFailed: string
    configuredFallback: string
  }
  home: {
    title: string
    subtitle: string
    newChat: string
    loading: string
    statusStrip: {
      pulse: string
      reflection: string
      activePlans: string
      agentRuns: string
      cronJobs: string
      diskPressure: string
      activeSessions: string
      never: string
      taskActive: (count: number) => string
      recent: string
      failed: (count: number) => string
      total: (count: number) => string
      gbFree: (gb: number) => string
      opsUnavailable: string
      touchedToday: (count: number) => string
    }
    pulseStates: { error: string; active: string; idle: string }
    reflectionStates: { failing: string; healthy: string; idle: string }
    plans: {
      title: string
      subtitle: string
      open: string
      empty: string
      executing: string
      doneSuffix: string
      activeSuffix: string
      updated: string
    }
    agentRuns: {
      title: string
      subtitle: string
      open: string
      empty: string
      agent: string
      tier: string
    }
    cron: {
      title: string
      subtitle: string
      open: string
      empty: string
      status: { failed: string; done: string; active: string; paused: string }
      nextRun: {
        completed: string
        paused: string
        nextTick: string
        cronSchedule: string
        after: (relative: string) => string
      }
    }
    sessions: {
      title: string
      subtitle: string
      open: string
      empty: string
      untitled: string
    }
    continue: {
      title: string
      subtitle: string
      untitled: string
      tasksTracked: (count: number) => string
      empty: string
    }
    notifications: {
      title: string
      unreadSuffix: (count: number) => string
      empty: string
    }
    recommendations: {
      title: string
      subtitle: string
      empty: string
      userMdTitle: string
      userMdDetail: string
      userMdAction: string
      anthropicKeyTitle: string
      anthropicKeyDetail: string
      anthropicKeyAction: string
      newChatTitle: string
      newChatDetail: string
      newChatAction: string
    }
    delivery: {
      title: string
      subtitle: string
      release: string
      devBuild: string
      localBuild: string
      pullRequests: string
      prsDetail: string
      openPRs: string
    }
    relativeTime: {
      never: string
      secondsAgo: (n: number) => string
      minutesAgo: (n: number) => string
      hoursAgo: (n: number) => string
      daysAgo: (n: number) => string
    }
    disk: { unknown: string; usedSuffix: string }
    errorLoad: string
    openSessionPlan: string
  }
}
