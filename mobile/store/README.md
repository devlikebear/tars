# App Store submission

What goes into App Store Connect for the iOS app (`com.devlikebear.tars.mobile`), kept here so a later version starts from the last one. Nothing in this folder is read by a build.

## Build and upload

```bash
scripts/mobile_release.sh 1.0.0 1            # archive, export, validate with App Store Connect
scripts/mobile_release.sh 1.0.0 1 --upload   # the same, then upload; shows under TestFlight
```

The script's header lists the environment it needs (team id, App Store Connect API key). The app record must exist in App Store Connect first: create it by hand with the bundle id above, then pass its numeric Apple ID as `ASC_APP_ID` for the first upload. Submitting a build for review, and releasing an approved one, are done in App Store Connect by a person.

The app is iPhone only (`TARGETED_DEVICE_FAMILY = 1`). An iPad runs it in iPhone compatibility mode. Adding iPad later is possible; removing it after a release is not.

## Screenshots

`screenshots/iphone-6.3/` holds captures from the iPhone 18 Pro simulator (1206 × 2622), release build, English, showing the demo desk. App Store Connect asks for the 6.9-inch size (1320 × 2868); the aspect ratio is the same to within two pixels, so scale them:

```bash
mkdir -p /tmp/tars-6.9 && for f in mobile/store/screenshots/iphone-6.3/*.png; do sips --resampleWidth 1320 "$f" --out /tmp/tars-6.9/up.png >/dev/null && sips --cropToHeightWidth 2868 1320 /tmp/tars-6.9/up.png --out "/tmp/tars-6.9/$(basename "$f")" >/dev/null; done; rm /tmp/tars-6.9/up.png
```

To take them again: build the release bundle (`cd mobile && npm run sync`), run it in the simulator with `-AppleLanguages "(en)"`, set the clock with `xcrun simctl status_bar <device> override --time 9:41`, add the demo desk, and capture with `xcrun simctl io <device> screenshot`.

## Listing

| Field | English (primary) | Korean |
|---|---|---|
| Name | TARS Remote | TARS 리모트 |
| Subtitle (30) | Approve your agent's work | 에이전트 작업을 폰에서 승인 |
| Category | Developer Tools | |
| Age rating | 4+ | |
| Price | Free | |
| Keywords (100) | `ai,agent,coding,developer,remote,approve,review,pull request,tailscale,self-hosted` | `AI,에이전트,코딩,개발자,원격,승인,리뷰,풀리퀘스트,셀프호스팅` |
| Support URL | `https://github.com/devlikebear/tars/issues` | |
| Marketing URL | `https://tars.marvin-42.com/` | |
| Privacy policy URL | where `privacy-policy.md` is published (not published yet) | |

"TARS" alone is likely taken as an app name; the name must be unique on the store, so check it when creating the record. The name on the home screen stays TARS.

**Promotional text (170)**

> Your coding agent stops and waits when it needs you. Answer from your phone: approve the plan, decide the review findings, merge the pull request.

> 코딩 에이전트는 사람이 필요할 때 멈춰서 기다립니다. 폰에서 답하세요: 계획 승인, 리뷰 지적 결정, 풀 리퀘스트 머지.

**Description (English)**

> TARS Remote is the phone companion to TARS, an open-source AI agent that runs on your own Mac or PC. TARS works through a task in stages (plan, build, review, pull request, merge) and stops at each gate until you answer. This app lets you answer from wherever you are.
>
> WHAT YOU CAN DO
> • Waiting: one list of everything that waits on you, across all your desktops.
> • Approve or change a plan before any code is written.
> • Decide each review finding: fix it or dismiss it. Ask a question about it first.
> • Approve the pull request draft, then the merge, with the CI results in front of you.
> • Answer a tool call that waits for permission.
> • Ideas: write an idea down any time. It stays on your phone until you send it to a desktop as a task.
>
> HOW IT CONNECTS
> The app talks only to desktops you pair yourself, over your own private network (Tailscale). Pair by scanning a QR code shown by TARS on the desktop. There is no account and no cloud service in between: your code and your conversations never pass through anyone else's server.
>
> TRY IT FIRST
> No desktop set up yet? Tap "Try the demo desk" to walk through sample tasks inside the app. Nothing is sent anywhere.
>
> REQUIREMENTS
> To use it with your own work you need TARS running on a Mac or Windows PC (free, https://tars.marvin-42.com) and Tailscale on both the desktop and this phone.

**Description (Korean)**

> TARS 리모트는 내 Mac이나 PC에서 돌아가는 오픈소스 AI 에이전트 TARS의 휴대폰용 동반 앱입니다. TARS는 작업을 단계별로(계획, 구현, 리뷰, 풀 리퀘스트, 머지) 진행하고, 단계마다 사람의 답을 기다리며 멈춥니다. 이 앱으로 어디서든 답할 수 있습니다.
>
> 할 수 있는 일
> • 대기 중: 연결한 모든 데스크톱에서 나를 기다리는 일을 한 목록으로 봅니다.
> • 코드를 쓰기 전에 계획을 승인하거나 고칩니다.
> • 리뷰 지적마다 고칠지 무시할지 정합니다. 정하기 전에 질문할 수도 있습니다.
> • CI 결과를 보면서 풀 리퀘스트 초안과 머지를 승인합니다.
> • 권한을 기다리는 도구 호출에 답합니다.
> • 아이디어: 언제든 아이디어를 적어 둡니다. 데스크톱에 작업으로 보내기 전까지는 휴대폰에만 남습니다.
>
> 연결 방식
> 앱은 직접 연결한 데스크톱하고만, 내 사설 네트워크(Tailscale)로 통신합니다. 데스크톱의 TARS가 보여 주는 QR 코드를 스캔해 연결합니다. 계정도, 중간 클라우드 서비스도 없습니다. 코드와 대화는 다른 누구의 서버도 거치지 않습니다.
>
> 먼저 써 보기
> 아직 데스크톱을 준비하지 않았다면 "데모 데스크톱 사용해 보기"를 눌러 앱 안의 예시 작업을 직접 진행해 볼 수 있습니다. 어디로도 전송되지 않습니다.
>
> 필요한 것
> 내 작업에 쓰려면 Mac 또는 Windows PC에서 TARS(무료, https://tars.marvin-42.com)가 실행 중이어야 하고, 데스크톱과 휴대폰 모두에 Tailscale이 필요합니다.

## App privacy answers

- Data collection: **Data Not Collected**. The developer receives nothing from the app: no analytics, no crash reporting SDK, no account.
- Tracking: none. The privacy manifest (`ios/App/App/PrivacyInfo.xcprivacy`) says the same.
- Export compliance: `ITSAppUsesNonExemptEncryption` is `false` in `Info.plist`. The app uses only the system's https and Keychain.

If a later version adds an account, analytics or a relay server, these answers change with it.

## Notes for App Review

Paste into "App Review Information → Notes". Attach one screen recording from a real iPhone that starts on the home screen and shows pairing with a desktop and answering a gate (App Review takes one attachment; join clips into one file).

> TARS Remote is a companion to TARS, an open-source AI coding agent that runs on the user's own Mac or PC (https://github.com/devlikebear/tars). It has no account and no server of ours: it connects only to desktops the user pairs, over the user's private Tailscale network. Because that network is private, you cannot reach a real desktop from your test device. The app therefore includes a built-in demo that exercises every feature with the app's real screens.
>
> HOW TO TEST
> 1. Open the app. Tap "Try the demo desk" at the bottom of the screen.
> 2. Waiting tab: three sample tasks wait. On "Fix the flaky upload retry", tap Approve on the waiting tool call.
> 3. Tap Open on "Add a dark mode toggle to Settings". Review the plan and tap Approve. After a few seconds of simulated work a review finding appears; tap Fix, then approve the pull request draft and the merge. The task finishes.
> 4. Tap "All tasks". Open "Fix the flaky upload retry": decide the two findings (Fix or Dismiss), or tap Ask to ask about one.
> 5. Ideas tab: type an idea and tap Save, then "Send as task". Choose Demo desk, a folder and a template, and tap Send. The new task appears under Waiting once its plan is drafted.
> 6. Desktops tab: the demo desk and its tasks. "Pair a desktop" opens the form a real pairing uses; it needs a desktop on the user's own network. The attached video shows a real pairing and a real task.
>
> The demo runs entirely inside the app: no network requests, no AI model, and its state resets when the app restarts.
>
> No sign-in is required and there is no demo account. The app has no purchases, ads, analytics or tracking, and behaves the same in every region. It does not generate content itself; the AI agent runs on the user's own computer with the user's own model provider.
