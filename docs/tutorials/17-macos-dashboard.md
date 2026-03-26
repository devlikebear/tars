# Step 17. macOS 대시보드

> 학습 목표: SwiftUI MenuBarExtra 앱으로 프로젝트 목록, 상세, 태스크 편집 UI를 구현

## 왜 macOS 대시보드가 필요한가

Step 16에서 프로젝트 저장소와 REST API를 만들었지만, 아직은 `curl`이나 터미널로만 데이터를 확인할 수 있습니다.

이번 Step에서는 메뉴바에서 바로 열리는 작은 macOS 앱을 만들어:

- 서버 연결 상태를 확인하고
- 프로젝트 목록을 보고
- 프로젝트 상세 보드와 활동 로그를 열고
- 프로젝트 생성과 태스크 추가/수정/삭제를 UI에서 처리합니다.

핵심은 “예쁜 앱”보다 **파일 기반 프로젝트 시스템을 눈으로 확인하고 조작하는 최소 대시보드**를 만드는 것입니다.

## 범위

Step 17에서는 다음까지만 다룹니다:

- `MenuBarExtra(.window)` 기반 macOS 앱
- 서버 연결/해제
- 프로젝트 목록/상세 조회
- 프로젝트 생성
- 태스크 추가/수정/삭제/상태 변경

다음 Step에서 붙는 것:

- Autopilot 시작/중지
- 승인/거절
- 실시간 SSE 고도화
- 산출물(deliverables) 표시 확장

현재 코드베이스에는 후속 Step UI 일부가 이미 같은 화면에 들어가 있습니다. 이 문서에서는 **프로젝트 대시보드의 기본 골격**에 집중합니다.

## 전체 구조

```text
dashboard/
├── Package.swift
└── Sources/
    ├── App.swift
    ├── APIClient.swift
    ├── Models.swift
    ├── ProjectListView.swift
    ├── CreateProjectView.swift
    ├── ProjectDetailView.swift
    ├── EditTaskSheet.swift
    └── BoardEditing.swift
```

역할 분리는 이렇게 잡습니다:

- `App.swift`
  - 연결 화면 / 목록 화면 / 상세 화면 전환
- `APIClient.swift`
  - `/health`, `/api/projects`, `/api/projects/{id}/board`, `/activity` 호출
- `ProjectListView.swift`
  - 목록과 생성 진입
- `ProjectDetailView.swift`
  - 보드, 활동 로그, 보조 패널
- `EditTaskSheet.swift`
  - 태스크 추가/수정/삭제
- `BoardEditing.swift`
  - 상태 전이와 배열 편집 같은 순수 로직

## 17-1. MenuBarExtra 앱 골격

**`dashboard/Sources/App.swift`**

메뉴바 앱의 화면은 3개면 충분합니다.

```swift
enum AppScreen {
    case connect
    case list
    case detail(String)
}
```

- `connect`
  - 서버 주소 입력
- `list`
  - 프로젝트 목록
- `detail`
  - 특정 프로젝트의 보드/활동

SwiftUI의 `MenuBarExtra(...).menuBarExtraStyle(.window)`를 쓰면 메뉴처럼 접히지 않고 작은 독립 창처럼 열립니다. Step 17의 대시보드에는 이 방식이 가장 단순합니다.

## 17-2. 서버 API 클라이언트

**`dashboard/Sources/APIClient.swift`**

대시보드는 결국 REST API 클라이언트입니다. 우선 필요한 호출은 5개입니다:

- `checkConnection()`
- `fetchProjects()`
- `fetchBoard(projectID:)`
- `fetchActivity(projectID:)`
- `createProject(name:objective:)`

태스크 편집은 별도 엔드포인트를 만들지 않고 **보드 전체를 PUT** 하는 방식으로 충분합니다.

```swift
func updateBoard(projectID: String, tasks: [BoardTask]) async {
    let body = tasks.map { ["ID": $0.id, "Title": $0.title, "Status": $0.status] }
    ...
}
```

왜 전체 보드를 다시 보내나?

- 서버 구현이 단순합니다
- Step 16의 파일 저장 구조와 잘 맞습니다
- “태스크 하나 수정”도 결국 `KANBAN.md` 전체를 다시 쓰는 작업입니다

## 17-3. 프로젝트 목록과 생성

**`dashboard/Sources/ProjectListView.swift`**

목록 화면은 다음 3가지만 있으면 됩니다:

- 새로고침 버튼
- 프로젝트 생성 버튼
- 프로젝트 행 탭으로 상세 이동

생성 팝오버는 별도 뷰로 분리합니다.

**`dashboard/Sources/CreateProjectView.swift`**

입력값:

- `name`
- `objective`

생성 성공 시:

1. `createProject()`
2. `fetchProjects()`
3. 팝오버 닫기
4. 생성된 프로젝트 상세 화면으로 이동

이 흐름 덕분에 사용자는 “만들고 나서 다시 목록에서 찾는” 단계를 건너뛸 수 있습니다.

## 17-4. 프로젝트 상세와 보드

**`dashboard/Sources/ProjectDetailView.swift`**

상세 화면의 핵심은 `BoardSection`입니다.

표시 요소:

- 현재 phase badge
- 칸반 컬럼별 태스크 개수
- 태스크 행 목록
- 최근 활동 로그

Step 17에서 태스크 편집 UX는 두 동작으로 나눕니다:

1. 왼쪽 상태 아이콘 클릭
   - `todo → in_progress → review → done` 순환
2. 연필 버튼 클릭
   - 제목/상태 수정
   - 삭제 가능

이렇게 하면 “빠른 상태 전이”와 “명시적 편집”을 분리할 수 있습니다.

## 17-5. 태스크 편집 Sheet

**`dashboard/Sources/EditTaskSheet.swift`**

이 뷰는 “추가”와 “수정”을 하나로 합칩니다.

```swift
init(client: APIClient, projectID: String, task: BoardTask? = nil, isPresented: Binding<Bool>)
```

- `task == nil`
  - 새 태스크 추가 모드
- `task != nil`
  - 기존 태스크 수정 모드

필드:

- `title`
- `status`

버튼:

- `Add` 또는 `Save`
- `Delete` (수정 모드에서만 표시)

이 패턴의 장점:

- 팝오버 UI를 재사용할 수 있음
- 추가/수정 로직이 같은 `updateBoard()`로 합쳐짐
- 상태 picker 덕분에 한 번에 원하는 컬럼으로 이동 가능

## 17-6. 순수 로직 분리

**`dashboard/Sources/BoardEditing.swift`**

UI 코드 안에 배열 수정 로직이 흩어지기 시작하면 금방 지저분해집니다. 그래서 순수 로직을 따로 뺍니다:

```swift
enum BoardEditing {
    static func nextStatus(after status: String) -> String
    static func upsert(tasks: [BoardTask], task: BoardTask) -> [BoardTask]
    static func delete(tasks: [BoardTask], id: String) -> [BoardTask]
}
```

이렇게 빼 두면 좋은 점:

- SwiftUI 뷰가 단순해짐
- 테스트를 먼저 쓸 수 있음
- Step 17 범위에서 “UI와 상태 변경 규칙”을 분리해 설명 가능

## 17-7. 테스트

Step 17은 SwiftUI 화면 자체를 스냅샷 테스트하지는 않지만, **보드 편집 규칙은 자동 테스트로 고정**합니다.

**`dashboard/Tests/BoardEditingTests.swift`**

검증 항목:

- 상태가 올바른 순서로 순환하는지
- 새 태스크가 append 되는지
- 기존 태스크 수정 시 같은 위치에서 교체되는지
- 삭제가 정확한 `id`에만 적용되는지

실행:

```bash
cd dashboard
swift test
```

서버 쪽 회귀 검증까지 같이 하려면:

```bash
go test ./...
```

## 체크포인트

- [x] 메뉴바에서 서버 연결 후 프로젝트 목록이 보인다
- [x] 프로젝트 상세에서 보드/활동이 표시된다
- [x] 대시보드에서 프로젝트를 생성할 수 있다
- [x] 대시보드에서 태스크를 추가/수정/삭제할 수 있다
- [x] 상태 아이콘으로 태스크 상태를 순환 변경할 수 있다

## 다음 단계

이제 파일 기반 프로젝트 저장소를 메뉴바 앱에서 직접 다룰 수 있습니다. 다음 Step에서는 같은 대시보드 위에 Autopilot 상태, 승인/거절, 실시간 업데이트를 붙여서 “감독 화면”으로 확장합니다.
