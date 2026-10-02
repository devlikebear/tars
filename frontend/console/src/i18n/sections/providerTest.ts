// Settings → Quick Start → LLM provider → Test connection. One result row per
// provider alias; aliases, kinds, versions and server details are content.
export const providerTestEn = {
  button: 'Test connection',
  testing: 'Testing…',
  failed: (message: string) => `Connection test failed: ${message}`,
  resultsLabel: 'Provider connection results',
  defaultBadge: 'default',
  status: {
    ok: 'OK',
    info: 'Unverified',
    warn: 'Warning',
    error: 'Failed',
  },
  noProviders: 'No providers are configured.',
  signedIn: (method: string) => (method ? `Signed in (${method})` : 'Signed in'),
  modelsAvailable: (count: number) => `${count} models available`,
  cliVersion: (version: string) => `CLI ${version}`,
  cliMissing: 'CLI not found on this machine.',
  notLoggedIn: 'The CLI is installed but not signed in. Run `claude` and log in.',
  authUnknown: 'CLI found. This version cannot report its sign-in, so that was not checked.',
  authCheckFailed: 'Could not list models. Sign in with an interactive `agy` session.',
  versionOld: (version: string, min: string) =>
    `agy ${version} is older than ${min}: turns run, but tool audit and cache tokens stay empty. Update the CLI.`,
  modelsFailed: 'Model listing failed.',
  modelsWarning: 'Model listing returned a warning.',
  modelsEmpty: 'Connected, but no models were listed.',
}

export type ProviderTestTranslations = typeof providerTestEn

export const providerTestKo: ProviderTestTranslations = {
  button: '연결 테스트',
  testing: '테스트 중…',
  failed: (message) => `연결 테스트 실패: ${message}`,
  resultsLabel: 'provider 연결 결과',
  defaultBadge: '기본',
  status: {
    ok: '정상',
    info: '미확인',
    warn: '주의',
    error: '실패',
  },
  noProviders: '설정된 provider가 없습니다.',
  signedIn: (method) => (method ? `로그인됨 (${method})` : '로그인됨'),
  modelsAvailable: (count) => `모델 ${count}개 사용 가능`,
  cliVersion: (version) => `CLI ${version}`,
  cliMissing: '이 컴퓨터에서 CLI를 찾지 못했습니다.',
  notLoggedIn: 'CLI는 설치됐지만 로그인되어 있지 않습니다. `claude`를 실행해 로그인하세요.',
  authUnknown: 'CLI를 찾았습니다. 이 버전은 로그인 상태를 알려 주지 않아 확인하지 않았습니다.',
  authCheckFailed: '모델 목록을 가져오지 못했습니다. 대화형 `agy`에서 로그인하세요.',
  versionOld: (version, min) =>
    `agy ${version}은(는) ${min}보다 오래됐습니다: 턴은 돌지만 도구 감사와 캐시 토큰이 비어 옵니다. CLI를 업데이트하세요.`,
  modelsFailed: '모델 목록을 가져오지 못했습니다.',
  modelsWarning: '모델 목록 조회가 경고를 반환했습니다.',
  modelsEmpty: '연결됐지만 모델 목록이 비어 있습니다.',
}
