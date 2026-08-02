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
    auth: {
      signOut: string
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
    sidebarSearchPlaceholder: string
    newChat: string
    filters: {
      all: string
      session: string
      main: string
      worker: string
      archived: string
    }
    sort: {
      recent: string
      name: string
      recentTitle: string
      nameTitle: string
    }
    loading: string
    sidebarLoading: string
    searchingTranscripts: string
    sidebarSearchingTranscripts: string
    empty: string
    noMatches: string
    sidebarEmpty: string
    sidebarNoMatches: string
    transcriptMatches: string
    autoTitle: string
    confirmDelete: string
    clickAgainToConfirm: string
    snippetFallbackKind: string
    relativeTime: {
      secondsAgo: (n: number) => string
      minutesAgo: (n: number) => string
      hoursAgo: (n: number) => string
      daysAgo: (n: number) => string
    }
    groups: {
      pinned: string
      recent: string
      older: string
    }
    actions: {
      rename: string
      autoTitle: string
      pin: string
      unpin: string
      archive: string
      restore: string
      compact: string
      delete: string
      confirm: string
      more: string
    }
    cleanup: {
      title: string
      count: (count: number) => string
      archiveSuggested: (count: number) => string
    }
    aiCleanup: {
      archiveTitle: string
      deleteTitle: string
      analyzeArchive: string
      analyzeDelete: string
      analyzing: string
      applyArchive: (count: number) => string
      applyDelete: (count: number) => string
      confirmDelete: (count: number) => string
      empty: string
      confidence: (confidence: number) => string
      source: (count: number, excluded: number) => string
    }
    errors: {
      loadFailed: string
      deleteFailed: string
      archiveFailed: string
      pinFailed: string
      aiCleanupFailed: string
      compactFailed: string
      compactSuccess: (count: number, percent: number) => string
      nothingToCompact: string
      generateTitleFailed: string
      transcriptSearchFailed: string
    }
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
    mode: {
      quick: string
      full: string
      switchToFull: string
      switchToQuick: string
      quickHint: string
      fullHint: string
    }
    steps: {
      provider: string
      tiers: string
      tools: string
      integrations: string
      channels: string
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
      renameNotice: (prev: string, next: string) => string
    }
    tools: {
      cardTitle: string
      cardMeta: string
      webSearchHeading: string
      webSearchEnableLabel: string
      webSearchEnableHint: string
      webSearchProviderLabel: string
      webSearchProviderPlaceholder: string
      webSearchApiKeyLabel: string
      webSearchApiKeyHint: string
      webSearchApiKeyPlaceholderKeep: string
      webSearchApiKeyPlaceholderNew: string
      webFetchHeading: string
      webFetchEnableLabel: string
      webFetchEnableHint: string
      webFetchPrivateHostsLabel: string
      webFetchPrivateHostsHint: string
      webFetchAllowlistLabel: string
      webFetchAllowlistPlaceholder: string
      webFetchAllowlistHint: string
      permissionsHeading: string
      highRiskUserLabel: string
      highRiskUserWarning: string
      backButton: string
      nextButton: string
      skipButton: string
    }
    integrations: {
      cardTitle: string
      cardMeta: string
      memoryHeading: string
      memoryProviderLabel: string
      memoryProviderHint: string
      memoryProviderPlaceholder: string
      memoryApiKeyLabel: string
      memoryApiKeyHint: string
      memoryApiKeyPlaceholderKeep: string
      memoryApiKeyPlaceholderNew: string
      memoryModelLabel: string
      memoryModelPlaceholder: string
      memoryBaseUrlLabel: string
      memoryBaseUrlPlaceholder: string
      memoryDimensionsLabel: string
      memoryDimensionsHint: string
      backButton: string
      nextButton: string
      skipButton: string
    }
    channels: {
      cardTitle: string
      cardMeta: string
      telegramHeading: string
      telegramEnableLabel: string
      telegramEnableHint: string
      telegramTokenLabel: string
      telegramTokenHint: string
      telegramTokenPlaceholderKeep: string
      telegramTokenPlaceholderNew: string
      telegramPollingLabel: string
      telegramPollingHint: string
      webhookHeading: string
      webhookEnableLabel: string
      webhookEnableHint: string
      restartHint: string
      backButton: string
      nextButton: string
      skipButton: string
    }
    complete: {
      title: string
      bodyQuick: string
      bodyFull: string
      configureMoreButton: string
      restartNowButton: string
      backToConsoleButton: string
      matrixHeading: string
      restartRequiredNotice: string
      rows: {
        provider: string
        tiers: string
        webSearch: string
        webFetch: string
        memoryEmbed: string
        telegram: string
        webhook: string
      }
      status: {
        ok: string
        missing: string
        skipped: string
      }
      jumpTo: (section: string) => string
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
    statusStrip: {
      pulseTicks: string
      lastTick: string
      unread: string
      neverTick: string
    }
    panels: {
      sessions: string
      sessionsTooltip: string
      files: string
      filesTooltip: string
      filesCount: (count: number) => string
      config: string
      configTooltip: string
      context: string
      contextTooltip: string
      prompt: string
      promptTooltip: string
      prior: string
      priorTooltip: string
      priorFull: string
      tasks: string
      tasksTooltip: string
      tasksProgressTooltip: (done: number, inProgress: number, pending: number) => string
      tasksCount: (done: number, total: number) => string
      git: string
      gitTooltip: string
      skills: string
      skillsTooltip: string
      skillsInbox: string
      cron: string
      cronTooltip: string
      health: string
      healthCount: (count: number) => string
      terminal: string
      dockEmpty: string
    }
    session: {
      newChat: string
      healthBadge: string
      healthBadgeTooltip: string
      actions: {
        rename: string
        aiTitle: string
        aiTitleTooltip: string
        compact: string
        compactTooltip: string
        copyAll: string
        copyAllTooltip: string
        download: string
        downloadTooltip: string
        extractSkill: string
        extractSkillTooltip: string
        delete: string
        confirmDelete: string
        zenEnter: string
        zenEnterTooltip: string
        zenExit: string
        zenExitTooltip: string
      }
    }
    planStrip: {
      label: string
      openTitle: string
      tasksSuffix: (completed: number, total: number) => string
      progressAria: (percent: number) => string
      activeTaskTooltip: (title: string) => string
    }
    feedback: {
      compacted: (count: number, original: number, final: number, percent: number) => string
      nothingToCompact: string
      compactFailed: string
      savedSkillDraft: (path: string) => string
      savedSkillDraftPlain: string
    }
    systemInit: {
      tars: string
      session: (idShort: string) => string
    }
    dropOverlay: string
    mention: {
      loading: string
      noMatches: string
    }
    toolbar: {
      attachFile: string
      attachImage: string
    }
    tierRecommendation: {
      ariaLabel: string
      chooseTierAria: string
      headline: (tier: string) => string
      tierLight: string
      tierStandard: string
      tierHeavy: string
    }
    input: {
      placeholderNew: string
      placeholderContinue: string
      send: string
      stop: string
    }
    message: {
      usageBadgeTitle: string
      usageIn: string
      usageOut: string
      usageCached: string
      copy: string
      copyTitle: string
      forkFromHere: string
      forkFromHereTitle: string
      reasoningSummary: string
      reasoningSummaryWithChars: (chars: number) => string
      roles: {
        system: string
        user: string
        assistant: string
        tool: string
      }
    }
  }
  configWizardCard: {
    kicker: string
    body: string
    button: string
  }
  config: {
    pageTitle: string
    viewToggleQuick: string
    viewToggleFields: string
    viewToggleYaml: string
    save: string
    saving: string
    discard: string
    changedSuffix: (count: number) => string
    viewChangesTooltip: string
    loading: string
    noConfigFile: string
    quickStartKicker: string
    quickStartTitle: string
    quickStartReady: (ready: number, total: number) => string
    defaultPrefix: (value: string) => string
    clickToToggle: string
    clickToEdit: string
    on: string
    off: string
    selectNone: string
    workspaceResetSuccess: (removed: number) => string
    failedResetWorkspace: string
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
    workers: {
      title: string
      subtitle: string
      ariaLabel: string
      refresh: string
      refreshing: string
      loading: string
      loadError: string
      enabled: string
      disabled: string
      protocol: string
      a2a: string
      disabledTitle: string
      disabledBody: string
      workersTitle: string
      noWorkers: string
      placementsTitle: string
      noPlacements: string
      eventsTitle: string
      noEvents: string
      lastSeen: string
      lease: string
      capabilities: string
      work: string
      step: string
      attempt: string
      sync: string
      checkpoint: string
      recovery: string
      egress: string
      resources: string
      updated: string
      published: string
      pendingPublish: string
      summary: {
        workers: string
        ready: string
        lost: string
        placements: string
        active: string
        recovering: string
        recoveries: string
      }
    }
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
  agentRuntime: {
    title: string
    subtitleRunDetail: string
    subtitleSubagents: string
    subtitleRuns: string
    back: string
    refresh: string
    loading: string
    analyzing: string
    recommendFromRuns: string
    newSubagent: string
    tabRuns: string
    tabSubagents: string
    tabsAriaLabel: string
    introEyebrow: string
    introTitle: string
    introBody: string
    toolStripAriaLabel: string
    toolDetail: {
      run: string
      orchestrate: string
      plan: string
    }
    filtersAriaLabel: string
    filterStatus: string
    filterTimeRange: string
    filterSearchLabel: string
    filterSearchPlaceholder: string
    apply: string
    statusAll: string
    statusRunning: string
    statusDone: string
    statusFailed: string
    range24h: string
    range7d: string
    rangeAll: string
    viewMode: {
      list: string
      tree: string
      gantt: string
      flow: string
    }
    visualAriaLabel: string
    costSummaryAriaLabel: string
    costToday: string
    cost7d: string
    costPlanTotals: string
    costLoadedRunCosts: string
    costNoData: string
    costRunSuffix: (n: number) => string
    emptyNoMatchEyebrow: string
    emptyNoMatchTitle: string
    emptyNoMatchBody: string
    clearFilters: string
    emptyNoRunsEyebrow: string
    emptyNoRunsTitle: string
    emptyNoRunsBody: string
    openChat: string
    sessionLink: (id: string) => string
    rowAgentDefault: string
    failedLoadRuns: string
    failedLoadSubagents: string
    failedLoadRun: string
    failedRestartRun: string
    failedUpdateTier: string
    failedRecommend: string
    failedDraft: string
    failedApplyDraft: string
    failedArchive: string
  }
  extensions: {
    title: string
    subtitle: string
    tabInstalled: string
    tabHub: string
    sandboxPassed: string
    sandboxFailed: string
    sandboxChecksSummary: (passed: number, total: number) => string
    createSkill: string
    createMCP: string
    diagnose: string
    diagnosing: string
    repair: string
    repairing: string
    reload: string
    reloading: string
    updateAll: string
    updating: string
    updatesAvailable: (count: number) => string
    loadingExtensions: string
    fetchingRegistry: string
    loading: string
    skillsTitle: string
    skillsDefinition: string
    pluginsTitle: string
    pluginsDeprecated: string
    pluginsAdvancedLegacy: string
    pluginsDeprecatedTooltip: string
    pluginsAdvancedTooltip: string
    pluginsDefinition: string
    pluginsPolicyNote: string
    mcpTitle: string
    mcpDefinition: string
    available: (n: number) => string
    noSkills: string
    noPlugins: string
    noMCP: string
    install: string
    installing: string
    update: string
    update_short: string
    updateBusy: string
    uninstall: string
    uninstalling: string
    installed: string
    enable: string
    disable: string
    on: string
    off: string
    detailLoading: string
    detailNoContent: string
    detailLoadFailed: string
    detailFallback: string
    invocableYes: (name: string) => string
    invocableNo: string
    detailMetaSource: string
    detailMetaInvocable: string
    detailMetaVersion: string
    toolsCount: (n: number) => string
    byAuthor: (author: string) => string
    versionPrefix: (v: string) => string
    updateTooltip: (v: string) => string
    failedLoadExtensions: string
    failedFetchRegistry: string
    installFailed: string
    hubSourceLabel: string
    hubSourceAll: string
    dryRunTitle: string
    dryRunSource: string
    dryRunSkill: string
    dryRunOriginURL: string
    dryRunOriginPath: string
    dryRunTargetDir: string
    dryRunLicense: string
    dryRunConvertedFrontmatter: string
    dryRunFiles: (count: number) => string
    dryRunAdapterWarnings: string
    dryRunChecksumWarnings: string
    dryRunChecksumMismatch: string
    dryRunAttribution: (label: string) => string
    dryRunAttributionNote: string
    dryRunCancel: string
    dryRunConfirm: string
    uninstallFailed: string
    toggleFailed: string
    reloadFailed: string
    updateFailed: string
    diagnosticsFailed: string
    repairFailed: string
    skillDeleteFailed: string
    skillUpdateFailed: string
    diagnosticsSuccess: (skills: number, mcps: number) => string
    repairSuccess: (name: string) => string
    healthPass: string
    healthWarn: string
    healthFail: string
    healthUnknown: string
    connected: string
    disconnected: string
    installSuccess: (kind: string, name: string, sandbox: string, plugin: string) => string
    sandboxSuffix: (summary: string) => string
    requiresPluginSuffix: (plugin: string) => string
    uninstallSuccess: (kind: string, name: string) => string
    toggledSuccess: (name: string, action: string) => string
    enabledLabel: string
    disabledLabel: string
    reloadSuccess: (skills: number, plugins: number, mcps: number) => string
    editSkill: string
    deleteSkill: string
    skillDeleteConfirm: (name: string) => string
    skillDeletedSuccess: (name: string) => string
    skillUpdatedSuccess: (name: string) => string
    skillCreatedSuccess: (path: string) => string
    mcpCreatedSuccess: (path: string) => string
    updatedTotal: (n: number) => string
    everythingUpToDate: string
    qualityScore: (score: number) => string
    qualityLabels: {
      lastUpdated: string
      testsPassing: string
      requiredTools: string
      permissions: string
      companionCli: string
      installs: string
      yes: string
      no: string
    }
  }
  sysprompt: {
    eyebrow: string
    title: string
    subtitle: string
    statWorkspaceFiles: string
    statAgentFiles: string
    statBuiltinTools: string
    failedLoadFiles: string
    failedLoadFile: string
    workspacePromptTitle: string
    loadingFiles: string
    groupWorkspaceIdentity: string
    groupAgentRules: string
    badgePresent: string
    badgeMissing: string
    editorTitle: string
    selectFilePrompt: string
    reload: string
    reloadPreview: string
    previewLoading: string
    showTechnicalDetails: string
    hideTechnicalDetails: string
    save: string
    saving: string
    savedSuccess: (path: string) => string
    failedSave: string
    failedLoadPreview: string
    badgeMainAgent: string
    badgeSubAgent: string
    existingFile: string
    missingFileStarter: string
    sectionLabel: (section: string, role: string) => string
    truncateWarning: (chars: number, max: number) => string
    impactLine: (size: string, tokens: number, chars: number, max: number, section: string, role: string) => string
    insertTemplateTitle: string
    insertTemplateDefaultDesc: string
    insertButton: string
    chooseTemplatePlaceholder: string
    starterAriaLabel: string
    templateInsertedSuccess: (label: string) => string
    selectFileEmpty: string
    diagnosticsTitle: string
    roleSemanticsLabel: string
    relevantToolsLabel: string
    noToolsDetected: string
    previewModalLabel: string
    previewKicker: string
    previewMainTitle: string
    previewSubTitle: string
    previewLoadingState: string
    previewClose: string
    tokensTotal: (n: number) => string
    tokensStatic: (n: number) => string
    tokensMemory: (n: number) => string
    tokensSuffix: (n: number) => string
    role: {
      USER: string
      IDENTITY: string
      AGENTS: string
      TOOLS: string
      PROJECT: string
      default: string
    }
    diagRoles: {
      USER: string
      IDENTITY: string
      PROJECT: string
      AGENTS: string
      TOOLS: string
    }
  }
  sessionLineage: {
    title: string
    subtitle: string
    refresh: string
    summaryAriaLabel: string
    graphAriaLabel: string
    statSessions: string
    statRoots: string
    statForks: string
    loading: string
    empty: string
    failedLoad: string
    timeUnknown: string
    forkPoint: string
    parentLabel: (name: string) => string
    indexLabel: (n: number) => string
    reviewInsights: string
    reviewLoading: string
    forkInsightsLabel: string
    queueSelected: (count: number) => string
    queuing: string
    queuedSummary: (promoted: number, skipped: number) => string
    failedQueue: string
    openMemoryInbox: string
    loadingForkInsights: string
    noForkInsights: string
    failedForkInsights: string
    provenance: string
    messageIndex: (n: number) => string
  }
  plans: {
    kicker: string
    title: string
    refresh: string
    failedLoad: string
    loadingPlans: string
    emptyTitle: string
    emptyBody: string
    openChat: string
    summaryAriaLabel: string
    plansAriaLabel: string
    activePlans: string
    inProgress: string
    pending: string
    completed: string
    readyToClose: string
    readyToCloseHint: string
    openPlan: string
    resolvePlan: string
    statusFallback: string
    updatedAt: (time: string) => string
    progressAria: (percent: number) => string
    doneSuffix: (done: number, total: number) => string
    activeSuffix: (n: number) => string
    pendingSuffix: (n: number) => string
    sessionKindWorker: string
    sessionKindSession: string
    timeNever: string
  }
  channels: {
    title: string
    telegramTitle: string
    policyLabel: (value: string) => string
    pollingOn: string
    pollingOff: string
    pollingLabel: (state: string) => string
    approveSection: string
    approveDescription: string
    approveCodePlaceholder: string
    approveButton: string
    approving: string
    enterCodeError: string
    approvedSuccess: (name: string) => string
    approveFailed: string
    pendingSection: string
    allowedSection: string
    loading: string
    noPending: string
    noAllowed: string
    failedLoad: string
    table: {
      code: string
      user: string
      chatId: string
      expires: string
      approved: string
    }
    revoke: string
    revoking: string
    revokeFailed: string
    accessRevoked: string
    dash: string
  }
}
