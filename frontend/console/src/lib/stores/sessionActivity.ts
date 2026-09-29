import { getChatActivity } from '../api/board.ts'
import { SessionActivityStore } from './sessionActivity.svelte'

function browserNotifications() {
  if (typeof window === 'undefined' || !('Notification' in window)) return null
  return {
    permission: () => Notification.permission,
    request: () => Notification.requestPermission(),
    show: (title: string, body: string, tag: string, onClick: () => void) => {
      const n = new Notification(title, { body, tag })
      n.onclick = () => {
        window.focus()
        onClick()
        n.close()
      }
    },
  }
}

function browserStorage() {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage
  } catch {
    return null
  }
}

export const sessionActivity = new SessionActivityStore(
  { getChatActivity },
  {
    now: () => Date.now(),
    storage: browserStorage(),
    notifications: browserNotifications(),
    visible: () => typeof document === 'undefined' || document.visibilityState === 'visible',
  },
)
