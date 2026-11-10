// The state of a confirm_memory question's answer buttons (tars#1001),
// from the candidate's inbox status. Pure, tested under Node.
import type { InitiativeMemoryAnswer } from './api/memory.ts'

export type MemoryQuestionState = 'loading' | 'open' | 'saved' | 'skipped' | 'handled' | 'failed'

export function memoryQuestionState(status: InitiativeMemoryAnswer['status'] | undefined): MemoryQuestionState {
  switch (status) {
    case undefined:
      return 'loading'
    case 'pending':
      return 'open'
    case 'approved':
      return 'saved'
    case 'rejected':
      return 'skipped'
    default:
      // merged on the Memory screen, or gone from the inbox.
      return 'handled'
  }
}
