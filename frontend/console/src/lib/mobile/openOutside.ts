// openOutside opens an address outside the app: in Safari on the iPhone,
// in a new tab in a development build in a desktop browser.
//
// In the Capacitor shell a top-level navigation to an address that is not
// the app's own is cancelled and handed to the system (Capacitor's
// WebViewDelegationHandler), so the app stays where it was. window.open is
// not used there: WKWebView refuses a window opened without a tap, and the
// address here arrives after several requests, long after the tap.
import { Capacitor } from '@capacitor/core'

export function openOutside(url: string): void {
  if (Capacitor.isNativePlatform()) {
    window.location.assign(url)
    return
  }
  window.open(url, '_blank', 'noopener')
}
