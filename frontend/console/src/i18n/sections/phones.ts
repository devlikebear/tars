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

  genericError: 'The request failed.',
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

  genericError: '요청이 실패했습니다.',
}
