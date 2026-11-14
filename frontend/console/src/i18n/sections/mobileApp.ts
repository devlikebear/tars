// The TARS mobile app (src/mobile/, docs/decisions/mobile-remote.md): its
// list of desktops and pairing with one.
export const mobileAppEn = {
  title: 'TARS',
  desktopsTitle: 'Desktops',
  empty: "No desktop yet. On a desktop, open Settings → Channels & Remote → Phones, choose Pair a phone, and scan the code with this phone's camera.",
  pairDesktop: 'Pair a desktop',
  refresh: 'Refresh',

  pairTitle: 'Pair with a desktop',
  pairHint: "Scanning the desk's QR code with the camera fills this in. You can also type what the desk shows under the code.",
  addressLabel: 'Desktop address',
  addressPlaceholder: 'mac-mini.your-tailnet.ts.net',
  codeLabel: 'Pairing code',
  phoneNameLabel: "This phone's name",
  phoneNameHint: "Shown in the desktop's list of phones.",
  defaultPhoneName: 'iPhone',
  pair: 'Pair',
  pairing: 'Pairing...',
  cancel: 'Cancel',
  pairedWith: (name: string) => `Paired with ${name}.`,

  failures: {
    invalid_address: "Enter the desktop's Tailscale address.",
    invalid_code: 'The code is wrong or expired. Make a new one at the desk.',
    locked: 'Too many tries. Wait a few minutes, then try again.',
    unreachable: "Can't reach this desktop. Is it awake, and is this phone on Tailscale?",
    failed: 'The desktop refused to pair.',
  },

  states: {
    checking: 'Checking',
    online: 'Online',
    unreachable: 'Unreachable',
    unpaired: 'Pair again',
    setup: 'Needs setup',
  },
  stateHints: {
    unreachable: 'Asleep, off, or off the tailnet.',
    unpaired: 'The desk revoked this phone. Pair again to use it.',
    setup: 'TARS on this desktop has no model set up yet.',
  } as Record<string, string>,

  forget: 'Forget',
  confirmForget: (name: string) => `Forget ${name} on this phone? The desktop keeps listing this phone until you revoke it there.`,
}

export type MobileAppTranslations = typeof mobileAppEn

export const mobileAppKo: MobileAppTranslations = {
  title: 'TARS',
  desktopsTitle: '데스크톱',
  empty: '아직 데스크톱이 없습니다. 데스크톱에서 설정 → 채널·원격 → 휴대폰을 열고 휴대폰 연결을 누른 뒤, 이 휴대폰 카메라로 코드를 스캔하세요.',
  pairDesktop: '데스크톱 연결',
  refresh: '새로고침',

  pairTitle: '데스크톱과 연결',
  pairHint: '카메라로 데스크톱의 QR 코드를 스캔하면 자동으로 채워집니다. 코드 아래에 보이는 내용을 직접 입력해도 됩니다.',
  addressLabel: '데스크톱 주소',
  addressPlaceholder: 'mac-mini.your-tailnet.ts.net',
  codeLabel: '연결 코드',
  phoneNameLabel: '이 휴대폰 이름',
  phoneNameHint: '데스크톱의 휴대폰 목록에 표시됩니다.',
  defaultPhoneName: 'iPhone',
  pair: '연결',
  pairing: '연결 중...',
  cancel: '취소',
  pairedWith: (name) => `${name}에 연결했습니다.`,

  failures: {
    invalid_address: '데스크톱의 Tailscale 주소를 입력하세요.',
    invalid_code: '코드가 틀렸거나 만료되었습니다. 데스크톱에서 새로 만드세요.',
    locked: '시도가 너무 많습니다. 몇 분 뒤에 다시 하세요.',
    unreachable: '이 데스크톱에 접속할 수 없습니다. 켜져 있고 이 휴대폰이 Tailscale에 연결되어 있나요?',
    failed: '데스크톱이 연결을 거절했습니다.',
  },

  states: {
    checking: '확인 중',
    online: '연결됨',
    unreachable: '접속 안 됨',
    unpaired: '다시 연결 필요',
    setup: '설정 필요',
  },
  stateHints: {
    unreachable: '잠자기 중이거나 꺼져 있거나 Tailscale에 없습니다.',
    unpaired: '데스크톱에서 이 휴대폰의 연결을 해제했습니다. 다시 연결하세요.',
    setup: '이 데스크톱의 TARS에 아직 모델이 설정되지 않았습니다.',
  },

  forget: '지우기',
  confirmForget: (name) => `이 휴대폰에서 ${name}을(를) 지울까요? 데스크톱에서 연결을 해제하기 전까지는 그쪽 목록에 이 휴대폰이 남아 있습니다.`,
}
