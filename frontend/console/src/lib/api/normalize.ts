import type {
  PlanArchiveResponse,
  GlobalPlansResponse,
  SessionTasks,
  TaskContract,
  TaskEvidence,
} from '../types'

export function normalizeSessionTasks(data: Partial<SessionTasks> | null | undefined): SessionTasks {
  return {
    ...(data?.plan ? { plan: data.plan } : {}),
    ...(data?.contract ? { contract: normalizeTaskContract(data.contract) } : {}),
    tasks: Array.isArray(data?.tasks)
      ? data.tasks.map((task) => ({
        ...task,
        evidence: Array.isArray(task.evidence) ? task.evidence.map(normalizeTaskEvidence) : [],
      }))
      : [],
  }
}

export function normalizeTaskEvidence(data: Partial<TaskEvidence> | null | undefined): TaskEvidence {
  return {
	...(data ?? {}),
    id: data?.id ?? '',
    type: data?.type ?? 'command_output_summary',
  }
}

export function normalizeTaskContract(data: Partial<TaskContract> | null | undefined): TaskContract {
  return {
    ...data,
    done_criteria: Array.isArray(data?.done_criteria) ? data.done_criteria : [],
    verification_commands: Array.isArray(data?.verification_commands) ? data.verification_commands : [],
    artifacts: Array.isArray(data?.artifacts) ? data.artifacts : [],
  }
}

export function normalizePlanArchive(data: Partial<PlanArchiveResponse> | null | undefined): PlanArchiveResponse {
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    count: typeof data?.count === 'number' ? data.count : Array.isArray(data?.items) ? data.items.length : 0,
  }
}

export function normalizeGlobalPlans(data: Partial<GlobalPlansResponse> | null | undefined): GlobalPlansResponse {
  return {
    items: Array.isArray(data?.items)
      ? data.items.map((item) => ({
        ...item,
        ...(item.contract ? { contract: normalizeTaskContract(item.contract) } : {}),
      }))
      : [],
    count: typeof data?.count === 'number' ? data.count : Array.isArray(data?.items) ? data.items.length : 0,
  }
}
