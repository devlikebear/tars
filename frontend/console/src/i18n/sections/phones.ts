// PhonesCard.svelte: pairing a phone, the paired phones and the folders a
// phone may start a focus task in (docs/decisions/mobile-remote.md §4),
// shown on the Channels & Remote config tab under the Tailscale card.
export const phonesEn = {
  ariaLabel: 'Paired phones',
  kicker: 'Mobile',
  title: 'Phones',
  intro: 'A paired phone starts and continues focus tasks on this desktop over Tailscale. It reaches focus mode only.',

  pair: 'Pair a phone',
  newCode: 'New code',
  working: 'Working...',
  scanHint: 'Scan with the TARS app. The code works once.',
  codeLabel: 'Pairing code',
  expiresIn: (time: string) => `Expires in ${time}`,
  expired: 'This code expired. Make a new one.',
  needsAddress: 'Turn on Tailscale remote access above first. A phone reaches this desktop only at its Tailscale address.',
  qrLabel: 'Pairing QR code',

  devicesTitle: 'Paired',
  noDevices: 'No phone is paired yet.',
  answeredOnPhone: (phone: number, total: number, pipelines: number) =>
    `Last 30 days: ${phone} of ${total} gate and card answers came from a phone${pipelines > 0 ? `, in ${pipelines} ${pipelines === 1 ? 'task' : 'tasks'}` : ''}.`,
  pairedOn: (date: string) => `Paired ${date}`,
  revoked: 'Revoked',
  revoke: 'Revoke',
  confirmRevoke: (name: string) => `Revoke ${name}? Its token stops working at once.`,

  foldersTitle: 'Folders a phone may start tasks in',
  foldersHint: 'A phone picks from this list and never sends a path of its own.',
  noFolders: 'None yet. A phone can continue tasks but not start one.',
  addFolder: 'Add folder',
  remove: 'Remove',
  missing: 'Not found',

  // Keeping this computer awake for a paired phone (desk_keep_awake.go).
  keepAwakeTitle: 'Keep this computer awake',
  keepAwakeLabel: 'Stay awake while plugged in and a phone is paired',
  keepAwakeHint: 'A phone reaches this desktop only while it is awake. On battery it sleeps as usual, and the display still turns off.',
  keepAwakeActive: 'Kept awake now.',
  keepAwakeNoPhone: 'On, but no phone is paired, so it may sleep.',
  keepAwakeFailed: 'On, but it could not be kept awake. It tries again within a minute.',
  keepAwakeUnsupported: 'Not available on this system yet: macOS only.',

  genericError: 'The request failed.',

  // On an iPhone, a pipeline's page (where a Telegram alert leads) offers to
  // open the same pipeline in the app (docs/decisions/mobile-remote.md §7).
  handoffText: 'Open this task in the TARS app?',
  handoffOpen: 'Open in app',
  handoffDismissAria: 'Stay in the browser',
}

export type PhonesTranslations = typeof phonesEn

export const phonesKo: PhonesTranslations = {
  ariaLabel: '연결된 휴대폰',
  kicker: '모바일',
  title: '휴대폰',
  intro: '연결한 휴대폰은 Tailscale로 이 데스크톱의 포커스 작업을 시작하고 이어서 할 수 있습니다. 포커스 모드에만 접근합니다.',

  pair: '휴대폰 연결',
  newCode: '새 코드',
  working: '처리 중...',
  scanHint: 'TARS 앱으로 스캔하세요. 코드는 한 번만 쓸 수 있습니다.',
  codeLabel: '연결 코드',
  expiresIn: (time) => `${time} 후 만료`,
  expired: '코드가 만료되었습니다. 새로 만드세요.',
  needsAddress: '먼저 위에서 Tailscale 원격 접속을 켜세요. 휴대폰은 이 데스크톱의 Tailscale 주소로만 접속합니다.',
  qrLabel: '연결용 QR 코드',

  devicesTitle: '연결됨',
  noDevices: '아직 연결된 휴대폰이 없습니다.',
  answeredOnPhone: (phone, total, pipelines) =>
    `지난 30일: 게이트·카드 답 ${total}건 중 ${phone}건을 휴대폰에서 했습니다${pipelines > 0 ? `(작업 ${pipelines}개)` : ''}.`,
  pairedOn: (date) => `${date} 연결`,
  revoked: '해제됨',
  revoke: '연결 해제',
  confirmRevoke: (name) => `${name} 연결을 해제할까요? 토큰이 바로 막힙니다.`,

  foldersTitle: '휴대폰이 작업을 시작할 수 있는 폴더',
  foldersHint: '휴대폰은 이 목록에서만 고르고 직접 경로를 보내지 않습니다.',
  noFolders: '아직 없습니다. 휴대폰은 작업을 이어서 할 수는 있지만 새로 시작할 수는 없습니다.',
  addFolder: '폴더 추가',
  remove: '빼기',
  missing: '없음',

  keepAwakeTitle: '이 컴퓨터 깨워 두기',
  keepAwakeLabel: '전원이 연결돼 있고 휴대폰이 연결된 동안 잠자지 않기',
  keepAwakeHint: '휴대폰은 이 데스크톱이 깨어 있을 때만 접속할 수 있습니다. 배터리로 동작할 때는 평소처럼 잠자고, 화면은 그대로 꺼집니다.',
  keepAwakeActive: '지금 깨워 두는 중입니다.',
  keepAwakeNoPhone: '켜져 있지만 연결된 휴대폰이 없어 잠잘 수 있습니다.',
  keepAwakeFailed: '켜져 있지만 깨워 두지 못했습니다. 1분 안에 다시 시도합니다.',
  keepAwakeUnsupported: '이 시스템에서는 아직 쓸 수 없습니다(macOS만 지원).',

  genericError: '요청이 실패했습니다.',

  handoffText: '이 작업을 TARS 앱에서 열까요?',
  handoffOpen: '앱에서 열기',
  handoffDismissAria: '브라우저에서 계속',
}
