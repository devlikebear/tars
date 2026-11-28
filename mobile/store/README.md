# App Store submission

What goes into App Store Connect for the iOS app (`com.devlikebear.tars.mobile`), kept here so a later version starts from the last one. Nothing in this folder is read by a build.

## Build and upload

```bash
scripts/mobile_release.sh 1.0.0 1            # archive, export, validate with App Store Connect
scripts/mobile_release.sh 1.0.0 1 --upload   # the same, then upload; shows under TestFlight
```

The record is "TARS Remote", Apple ID `6835098160`. The listing text, screenshots, review notes, age rating, price and availability below were entered through the App Store Connect API; the app privacy answers have no API and were set on the web. The script's header lists the environment it needs (team id, App Store Connect API key). The app record must exist in App Store Connect first: create it by hand with the bundle id above, then pass its numeric Apple ID as `ASC_APP_ID` for the first upload. Submitting a build for review, and releasing an approved one, are done in App Store Connect by a person.

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
| Subtitle (30) | Make apps from your phone | 폰으로 앱 만들기 |
| Category | Developer Tools | |
| Age rating | 4+ | |
| Price | Free | |
| Keywords (100) | `ai,app builder,no code,make app,web app,agent,self-hosted,tailscale,prototype,claude` | `AI,앱 만들기,노코드,웹앱,에이전트,바이브코딩,셀프호스팅,프로토타입` |
| Support URL | `https://github.com/devlikebear/tars/issues` | |
| Marketing URL | `https://tars.marvin-42.com/` | |
| Privacy policy URL | `https://github.com/devlikebear/tars/blob/main/mobile/store/privacy-policy.md` | |

"TARS" alone is likely taken as an app name; the name must be unique on the store, so check it when creating the record. The name on the home screen stays TARS.

**Promotional text (170)**

> Tell your computer what to make, in a sentence. A team of AI helpers plans it, makes it and checks it. Answer their questions and try the app from your phone.

> 만들고 싶은 걸 한두 문장으로 말하세요. AI 팀이 계획하고, 만들고, 확인합니다. 폰에서 팀의 질문에 답하고 만든 앱을 써 보세요.

**What's new in 1.1.0**

> A new app for making things. Start a project from your phone with an idea in a sentence or two, answer the team's questions, approve the blueprint, follow the making and the checks, then try the finished app in Safari. Ask for changes, go back to an earlier version, rename or archive a project. Pair a computer from its Settings screen.

> 무언가를 만드는 앱으로 새로 바뀌었습니다. 폰에서 한두 문장의 아이디어로 프로젝트를 시작하고, 팀의 질문에 답하고, 설계도를 승인하고, 만들기와 확인 과정을 지켜본 뒤 완성된 앱을 Safari에서 써 보세요. 바꿔 달라고 요청하거나, 이전 버전으로 되돌리거나, 프로젝트 이름을 바꾸고 보관할 수 있습니다. 컴퓨터는 그 컴퓨터의 설정 화면에서 연결합니다.

**Description (English)**

> TARS Remote is the phone app for TARS, a free, open-source maker that runs on your own Mac. Describe what you want in a sentence or two — a sign-up page, a to-do list, a page for your café — and a team of AI helpers on your computer plans it, makes it and checks it. You don't need to read or write code.
>
> WHAT YOU CAN DO
> • Start a project from your phone: write the idea, pick the computer, send it.
> • Answer the leader's questions, or let the team decide.
> • See the blueprint — the screens, what gets saved, and how it will be checked — and approve it or say what to change.
> • Follow the work: what the team is making and which checks pass.
> • Try it: open the finished app in Safari on your phone.
> • Ask for a change in your own words, go back to an earlier version, rename or archive a project.
> • See every project on every computer you've paired, with the ones waiting on you first.
>
> HOW IT CONNECTS
> The app talks only to computers you pair yourself, over your own private network (Tailscale). Pair by scanning the QR code in TARS's Settings on your computer. There is no account and no cloud service in between: your projects never pass through anyone else's server.
>
> TRY IT FIRST
> No computer set up yet? On the Computers tab, tap "Try the sample computer" and make something from + New. It all happens inside the app; nothing is sent anywhere.
>
> REQUIREMENTS
> To make real apps you need TARS running on a Mac (free, https://tars.marvin-42.com) with Claude Code, and Tailscale on both the Mac and this phone.

**Description (Korean)**

> TARS 리모트는 내 Mac에서 돌아가는 무료 오픈소스 제작 도구 TARS의 휴대폰 앱입니다. 만들고 싶은 걸 한두 문장으로 말하면 — 신청 페이지, 할 일 목록, 카페 소개 페이지 — 내 컴퓨터의 AI 팀이 계획하고, 만들고, 확인합니다. 코드를 읽거나 쓸 줄 몰라도 됩니다.
>
> 할 수 있는 일
> • 폰에서 프로젝트 시작: 아이디어를 적고, 컴퓨터를 고르고, 보내면 됩니다.
> • 리더봇의 질문에 답하거나, 팀이 정하게 맡깁니다.
> • 설계도(화면, 저장되는 것, 확인 방법)를 보고 승인하거나 바꿀 점을 말합니다.
> • 팀이 무엇을 만들고 있는지, 어떤 확인을 통과했는지 봅니다.
> • 써 보기: 완성된 앱을 폰의 Safari에서 엽니다.
> • 내 말로 수정을 요청하고, 이전 버전으로 되돌리고, 프로젝트 이름을 바꾸거나 보관합니다.
> • 연결한 모든 컴퓨터의 프로젝트를 한곳에서, 나를 기다리는 것부터 봅니다.
>
> 연결 방식
> 앱은 직접 연결한 컴퓨터하고만, 내 사설 네트워크(Tailscale)로 통신합니다. 컴퓨터의 TARS 설정 화면에 있는 QR 코드를 스캔해 연결합니다. 계정도, 중간 클라우드 서비스도 없습니다. 프로젝트는 다른 누구의 서버도 거치지 않습니다.
>
> 먼저 써 보기
> 아직 컴퓨터를 준비하지 않았다면 컴퓨터 탭에서 "연습용 컴퓨터로 해 보기"를 누르고 + 새로 만들기에서 만들어 보세요. 모두 앱 안에서 일어나고, 어디로도 전송되지 않습니다.
>
> 필요한 것
> 실제 앱을 만들려면 Mac에서 TARS(무료, https://tars.marvin-42.com)와 Claude Code가 실행 중이어야 하고, Mac과 휴대폰 모두에 Tailscale이 필요합니다.

## App privacy answers

- Data collection: **Data Not Collected**. The developer receives nothing from the app: no analytics, no crash reporting SDK, no account.
- Tracking: none. The privacy manifest (`ios/App/App/PrivacyInfo.xcprivacy`) says the same.
- Export compliance: `ITSAppUsesNonExemptEncryption` is `false` in `Info.plist`. The app uses only the system's https and Keychain.

If a later version adds an account, analytics or a relay server, these answers change with it.

## Notes for App Review

For 1.1.0 (drafted 2026-11-28; 1.0.0's are in git history). They go in "App Review Information → Notes", with one screen recording from a real iPhone attached that shows pairing with a computer and a real project made, tried and changed (App Review takes one attachment; join clips into one file, and H.264 at 720 wide keeps it to a few megabytes).

> TARS Remote is the phone app for TARS, an open-source AI app maker that runs on the user's own Mac (https://github.com/devlikebear/tars). It has no account and no server of ours: it connects only to computers the user pairs, over the user's private Tailscale network. Because that network is private, you cannot reach a real computer from your test device. The app therefore includes a sample computer that runs every feature with the app's real screens.
>
> HOW TO TEST
> 1. Open the app. On the Computers tab, tap "Try the sample computer".
> 2. Tap "+ New". Type an idea, for example "A to-do list I can check off", and send it to the sample computer.
> 3. The project opens. The team's leader asks a question or two: pick an answer, or tap "You decide".
> 4. Open the Blueprint tab: it lists the screens, what gets saved and the checks. Tap Approve.
> 5. Follow Progress and Check results for about a minute: the team makes the app, one check fails at first, the team fixes it, and the project finishes.
> 6. Open "My app" and tap "Try it". On the sample computer it explains that there is no real app to open; on a real computer it opens the app in Safari, as the attached recording shows.
> 7. Ask for a change in your own words, open Versions and go back to the first version, then rename or archive the project from the Projects tab.
>
> The attached recording, from a real iPhone (the interface is in Korean there), shows the real thing: pairing by scanning the QR code in TARS's Settings on a Mac, a project started from the phone, the blueprint approved, the finished app opened with Try it, and a change made.
>
> The sample computer runs entirely inside the app: no network requests, no AI model, and its state resets when the app restarts.
>
> No sign-in is required and there is no demo account. The app has no purchases, ads, analytics or tracking, and behaves the same in every region. It does not generate content itself; the AI runs on the user's own computer with the user's own model provider.
