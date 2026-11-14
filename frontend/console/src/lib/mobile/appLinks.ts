// How the mobile app hears from the system: links it is opened with (a
// scanned tars-mobile://pair QR code) and coming back to the foreground.
// The native shell and a desktop browser fill this in differently
// (src/mobile/main.ts), so screens take it as a prop.
export type AppLinks = {
  // Calls back with each link the app is opened with, including the one
  // that launched it; returns a function that stops listening.
  listen(onLink: (url: string) => void): () => void
  // Calls back each time the app comes back to the foreground.
  onResume(callback: () => void): () => void
}
