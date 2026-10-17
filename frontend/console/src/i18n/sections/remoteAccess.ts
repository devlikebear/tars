// RemoteAccessCard.svelte (#1241): the Tailscale Serve remote-access card
// shown on the Channels & Remote config tab. `checks` is keyed by the
// server's stable check code (RemoteAccessCheck.key, e.g.
// "tailscale_installed") so the card never renders server prose directly.
export const remoteAccessEn = {
  ariaLabel: 'Remote access',
  kicker: 'Remote Access',
  title: 'Tailscale',

  status: {
    checking: 'Checking',
    serving: 'Serving',
    portInUse: 'Port in use',
    idle: 'Idle',
    notReady: 'Not ready',
  },

  checkingTailscale: 'Checking Tailscale status...',
  tailscaleLoggedIn: 'Tailscale: logged in',
  tailscaleLoggedOut: 'Tailscale: logged out',
  tailscaleNotInstalled: 'Tailscale: not installed',
  device: (name: string) => `Device: ${name}`,
  url: (url: string) => `URL: ${url}`,
  https: (port: number) => `HTTPS: ${port}`,
  copy: 'Copy',
  urlCopied: 'URL copied.',

  httpsPortLabel: 'HTTPS port',
  enable: 'Enable',
  disable: 'Disable',
  refresh: 'Refresh',
  working: 'Working...',
  enabled: 'Remote access enabled.',
  disabled: 'Remote access disabled.',

  adminPasswordLabel: 'Admin password',
  userPasswordLabel: 'User password',
  save: 'Save',
  passwordSaved: (role: 'admin' | 'user'): string => (role === 'admin' ? 'Admin password saved.' : 'User password saved.'),

  genericError: 'Remote access request failed.',

  checks: {
    api_auth_required: 'API auth mode must be set to required before exposing the console through remote access.',
    admin_password_configured: 'An admin browser login password must be configured.',
    user_password_configured: 'A user browser login password must be configured.',
    tailscale_installed: 'The Tailscale CLI must be installed on this Mac.',
    tailscale_logged_in: 'Tailscale must be logged in before remote access can be enabled.',
    serve_target_available: 'The configured Tailscale HTTPS port must be idle, or already in use by TARS.',
  } as Record<string, string>,
}

export type RemoteAccessTranslations = typeof remoteAccessEn

export const remoteAccessKo: RemoteAccessTranslations = {
  ariaLabel: '원격 접속',
  kicker: '원격 접속',
  title: 'Tailscale',

  status: {
    checking: '확인 중',
    serving: '제공 중',
    portInUse: '포트 사용 중',
    idle: '대기 중',
    notReady: '준비 안 됨',
  },

  checkingTailscale: 'Tailscale 상태 확인 중...',
  tailscaleLoggedIn: 'Tailscale: 로그인됨',
  tailscaleLoggedOut: 'Tailscale: 로그아웃됨',
  tailscaleNotInstalled: 'Tailscale: 설치 안 됨',
  device: (name) => `기기: ${name}`,
  url: (url) => `주소: ${url}`,
  https: (port) => `HTTPS 포트: ${port}`,
  copy: '복사',
  urlCopied: '주소를 복사했습니다.',

  httpsPortLabel: 'HTTPS 포트',
  enable: '활성화',
  disable: '비활성화',
  refresh: '새로고침',
  working: '처리 중...',
  enabled: '원격 접속을 활성화했습니다.',
  disabled: '원격 접속을 비활성화했습니다.',

  adminPasswordLabel: '관리자 비밀번호',
  userPasswordLabel: '사용자 비밀번호',
  save: '저장',
  passwordSaved: (role) => (role === 'admin' ? '관리자 비밀번호를 저장했습니다.' : '사용자 비밀번호를 저장했습니다.'),

  genericError: '원격 접속 요청이 실패했습니다.',

  checks: {
    api_auth_required: '원격 접속을 열기 전에 API 인증 모드를 required로 설정해야 합니다.',
    admin_password_configured: '관리자 브라우저 로그인 비밀번호를 설정해야 합니다.',
    user_password_configured: '사용자 브라우저 로그인 비밀번호를 설정해야 합니다.',
    tailscale_installed: '이 Mac에 Tailscale이 설치되어 있어야 합니다.',
    tailscale_logged_in: '원격 접속을 켜려면 먼저 Tailscale에 로그인해야 합니다.',
    serve_target_available: 'Tailscale이 쓰는 HTTPS 포트가 비어 있거나 TARS가 이미 쓰고 있어야 합니다.',
  },
}
