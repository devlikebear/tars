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
}
