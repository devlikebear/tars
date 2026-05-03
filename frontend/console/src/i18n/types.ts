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
}
