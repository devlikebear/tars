// Entry of the TARS mobile app's web bundle (docs/decisions/mobile-remote.md
// §1): a second Vite entry of the console (vite.mobile.config.ts), built into
// mobile/www and loaded by the Capacitor shell in mobile/.
import { mount } from 'svelte'
import { App } from '@capacitor/app'
import { Capacitor } from '@capacitor/core'
import '../app.css'
import './mobile.css'
import MobileApp from './MobileApp.svelte'
import type { AppLinks } from '../lib/mobile/appLinks'
import { desktopAPI, eventsAPI } from '../lib/mobile/desktopApi'
import { createDemoDesk, demoBaseURL } from '../lib/mobile/demoDesk'
import { DesktopEvents } from '../lib/mobile/desktopEvents'
import { DesktopStore } from '../lib/mobile/desktopStore.svelte'
import { IdeaStore } from '../lib/mobile/ideaStore.svelte'
import { ProjectStore } from '../lib/mobile/projectStore.svelte'
import { appSecureStore } from '../lib/mobile/secureStore'
import { serveLocally } from '../lib/api/client'

// Desktops with their tokens and the saved ideas both live in the Keychain:
// one store the app owns, which iOS does not clear under storage pressure
// the way it may clear a web view's localStorage.
const storage = appSecureStore()

// The demo desk answers inside the app; it is only reached once someone
// adds it from the Desktops tab.
serveLocally(demoBaseURL, createDemoDesk().handle)

const store = new DesktopStore({
  storage,
  api: desktopAPI,
  // A development build pairs with a server on this Mac over loopback http
  // (the Simulator); a release build only with a Tailscale https address.
  allowLoopbackHTTP: import.meta.env.DEV,
})

const links: AppLinks = Capacitor.isNativePlatform()
  ? {
      listen(onLink) {
        void App.getLaunchUrl().then((launch) => launch?.url && onLink(launch.url))
        const handle = App.addListener('appUrlOpen', (event) => onLink(event.url))
        return () => void handle.then((h) => h.remove())
      },
      onResume(callback) {
        const handle = App.addListener('resume', callback)
        return () => void handle.then((h) => h.remove())
      },
    }
  : {
      // In a desktop browser (vite dev), ?pair=<link> stands in for a scan.
      listen(onLink) {
        const link = new URLSearchParams(window.location.search).get('pair')
        if (link) onLink(link)
        return () => {}
      },
      onResume(callback) {
        const onVisible = () => document.visibilityState === 'visible' && callback()
        document.addEventListener('visibilitychange', onVisible)
        return () => document.removeEventListener('visibilitychange', onVisible)
      },
    }

const app = mount(MobileApp, {
  target: document.getElementById('app')!,
  props: {
    store,
    projects: new ProjectStore(),
    ideas: new IdeaStore({ storage }),
    events: new DesktopEvents({ api: eventsAPI }),
    links,
  },
})

export default app
