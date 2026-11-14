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
import { desktopAPI } from '../lib/mobile/desktopApi'
import { DesktopStore } from '../lib/mobile/desktopStore.svelte'
import { appSecureStore } from '../lib/mobile/secureStore'

const store = new DesktopStore({
  storage: appSecureStore(),
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
  props: { store, links },
})

export default app
