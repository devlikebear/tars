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

  refreshing: 'Checking...',
  tabs: { inbox: 'Waiting', desktops: 'Desktops' },
  inboxNoDesktops: 'Pair a desktop to see what waits on you there.',
  inboxChecking: 'Asking your desktops...',
  inboxEmpty: 'Nothing waits on you.',
  inboxEmptyHint: 'A focus pipeline shows here when a gate opens, a card needs a decision, or a tool call waits for an answer.',
  notShown: (names: string) => `Not shown: ${names} did not answer.`,
  gates: {
    plan: 'Plan ready for approval (G1)',
    triage: 'Findings to triage',
    pr: 'Ready to open a pull request (G3)',
    merge: 'Ready to merge (G4)',
    blocked: 'Blocked: needs a decision',
  } as Record<string, string>,
  cardsToDecide: (n: number) => (n === 1 ? '1 card to decide' : `${n} cards to decide`),
  toolCall: 'Tool call waiting',
  approve: 'Approve',
  reject: 'Reject',
  allowOnce: 'Allow once',
  deny: 'Deny',
  answerFailures: {
    gone: 'Already answered, or it timed out.',
    failed: "Couldn't send the answer. Try again.",
  },
  openOnDesktop: 'Open in the desktop console',
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

  refreshing: '확인 중...',
  tabs: { inbox: '대기 중', desktops: '데스크톱' },
  inboxNoDesktops: '데스크톱을 연결하면 그곳에서 기다리는 일이 여기에 보입니다.',
  inboxChecking: '데스크톱에 묻는 중...',
  inboxEmpty: '기다리는 일이 없습니다.',
  inboxEmptyHint: '게이트가 열리거나, 카드에 결정이 필요하거나, 도구 호출이 답을 기다리면 포커스 파이프라인이 여기에 나타납니다.',
  notShown: (names) => `표시 안 됨: ${names}이(가) 응답하지 않았습니다.`,
  gates: {
    plan: '계획 승인 대기 (G1)',
    triage: '분류할 지적 사항',
    pr: '풀 리퀘스트 열기 대기 (G3)',
    merge: '머지 대기 (G4)',
    blocked: '막힘: 결정 필요',
  },
  cardsToDecide: (n) => `결정할 카드 ${n}개`,
  toolCall: '도구 호출 대기',
  approve: '승인',
  reject: '거절',
  allowOnce: '한 번 허용',
  deny: '거부',
  answerFailures: {
    gone: '이미 답했거나 시간이 지났습니다.',
    failed: '답을 보내지 못했습니다. 다시 시도하세요.',
  },
  openOnDesktop: '데스크톱 콘솔에서 열기',
}
