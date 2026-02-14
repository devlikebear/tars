import React, {useCallback, useMemo, useState} from 'react';
import {Box, Text, render, useApp, useInput} from 'ink';
import TextInput from 'ink-text-input';

type Role = 'user' | 'assistant' | 'error' | 'system';

type ChatLine = {
  role: Role;
  text: string;
};

type CliOptions = {
  serverUrl: string;
  sessionId: string;
  verbose: boolean;
};

type ChatSSEEvent = {
  type?: string;
  text?: string;
  error?: string;
  session_id?: string;
  phase?: string;
  message?: string;
  tool_name?: string;
};

function parseArgs(argv: string[]): CliOptions {
  let serverUrl = 'http://127.0.0.1:8080';
  let sessionId = '';
  let verbose = false;

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i] ?? '';
    if (arg === '--verbose') {
      verbose = true;
      continue;
    }
    if (arg === '--server-url' && argv[i + 1]) {
      serverUrl = argv[i + 1]!;
      i += 1;
      continue;
    }
    if (arg.startsWith('--server-url=')) {
      serverUrl = arg.slice('--server-url='.length);
      continue;
    }
    if (arg === '--session' && argv[i + 1]) {
      sessionId = argv[i + 1]!;
      i += 1;
      continue;
    }
    if (arg.startsWith('--session=')) {
      sessionId = arg.slice('--session='.length);
    }
  }

  return {serverUrl, sessionId, verbose};
}

function appendBounded(lines: string[], next: string, max: number): string[] {
  const trimmed = next.trim();
  if (trimmed === '') {
    return lines;
  }
  const out = [...lines, trimmed];
  if (out.length <= max) {
    return out;
  }
  return out.slice(out.length - max);
}

async function streamChat(params: {
  serverUrl: string;
  sessionId: string;
  message: string;
  onStatus: (line: string) => void;
  onDelta: (text: string) => void;
  onDone: (sessionId: string) => void;
  onDebug: (line: string) => void;
}): Promise<void> {
  const endpoint = `${params.serverUrl.replace(/\/+$/, '')}/v1/chat`;
  const payload: Record<string, string> = {message: params.message};
  if (params.sessionId.trim() !== '') {
    payload.session_id = params.sessionId.trim();
  }

  params.onDebug(`POST ${endpoint}`);
  const resp = await fetch(endpoint, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(payload)
  });

  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(`chat endpoint status ${resp.status}: ${body.trim()}`);
  }
  if (!resp.body) {
    throw new Error('empty response body');
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder('utf-8');
  let buffer = '';

  const consumeLine = (lineRaw: string): {done: boolean; nextSessionId?: string} => {
    const line = lineRaw.trim();
    if (!line.startsWith('data:')) {
      return {done: false};
    }
    const payloadText = line.slice('data:'.length).trim();
    if (payloadText.length === 0) {
      return {done: false};
    }

    let evt: ChatSSEEvent;
    try {
      evt = JSON.parse(payloadText) as ChatSSEEvent;
    } catch (err) {
      throw new Error(`decode sse event: ${String(err)}`);
    }

    if (evt.type === 'status') {
      let status = (evt.message ?? '').trim();
      if (status === '') {
        status = (evt.phase ?? '').trim();
      }
      if ((evt.tool_name ?? '').trim() !== '') {
        status = `${status} (${evt.tool_name!.trim()})`;
      }
      params.onStatus(status);
      return {done: false};
    }

    if (evt.type === 'delta') {
      params.onDelta(evt.text ?? '');
      return {done: false};
    }

    if (evt.type === 'error') {
      throw new Error((evt.error ?? 'chat stream error').trim());
    }

    if (evt.type === 'done') {
      return {done: true, nextSessionId: (evt.session_id ?? '').trim()};
    }

    return {done: false};
  };

  while (true) {
    const {value, done} = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, {stream: true});
    const parts = buffer.split('\n');
    buffer = parts.pop() ?? '';
    for (const part of parts) {
      const outcome = consumeLine(part);
      if (outcome.done) {
        if ((outcome.nextSessionId ?? '') !== '') {
          params.onDone(outcome.nextSessionId!);
        } else {
          params.onDone('');
        }
        return;
      }
    }
  }
}

function App({initial}: {initial: CliOptions}): React.JSX.Element {
  const {exit} = useApp();
  const [sessionId, setSessionId] = useState<string>(initial.sessionId);
  const [input, setInput] = useState<string>('');
  const [busy, setBusy] = useState<boolean>(false);
  const [messages, setMessages] = useState<ChatLine[]>([]);
  const [assistantDraft, setAssistantDraft] = useState<string>('');
  const [statusLines, setStatusLines] = useState<string[]>([]);
  const [debugLines, setDebugLines] = useState<string[]>([]);

  const pushStatus = useCallback((line: string) => {
    setStatusLines((prev) => appendBounded(prev, line, 200));
  }, []);

  const pushDebug = useCallback((line: string) => {
    if (!initial.verbose) {
      return;
    }
    setDebugLines((prev) => appendBounded(prev, line, 200));
  }, [initial.verbose]);

  const submit = useCallback(async () => {
    const text = input.trim();
    if (text === '' || busy) {
      return;
    }

    setInput('');
    setBusy(true);
    setAssistantDraft('');
    setMessages((prev) => [...prev, {role: 'user', text}]);

    try {
      await streamChat({
        serverUrl: initial.serverUrl,
        sessionId,
        message: text,
        onStatus: (line) => {
          pushStatus(line);
          pushDebug(`status: ${line}`);
        },
        onDelta: (chunk) => {
          if (chunk !== '') {
            setAssistantDraft((prev) => prev + chunk);
          }
        },
        onDone: (nextSession) => {
          if (assistantDraft.trim() !== '') {
            setMessages((prev) => [...prev, {role: 'assistant', text: assistantDraft}]);
            setAssistantDraft('');
          }
          if (nextSession !== '') {
            setSessionId(nextSession);
          }
        },
        onDebug: pushDebug
      });
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      setMessages((prev) => [...prev, {role: 'error', text: message}]);
      pushStatus(`error: ${message}`);
      pushDebug(`error: ${message}`);
    } finally {
      if (assistantDraft.trim() !== '') {
        const finalText = assistantDraft;
        setMessages((prev) => [...prev, {role: 'assistant', text: finalText}]);
        setAssistantDraft('');
      }
      setBusy(false);
    }
  }, [assistantDraft, busy, initial.serverUrl, input, pushDebug, pushStatus, sessionId]);

  useInput((key, data) => {
    if (data.ctrl && key === 'c') {
      exit();
    }
  });

  const visibleMessages = useMemo(() => messages.slice(-30), [messages]);
  const visibleStatus = useMemo(() => statusLines.slice(-20), [statusLines]);
  const visibleDebug = useMemo(() => debugLines.slice(-20), [debugLines]);

  return (
    <Box flexDirection="column">
      <Box marginBottom={1}>
        <Text color="cyan">tars-ui</Text>
        <Text>  server=</Text>
        <Text color="green">{initial.serverUrl}</Text>
        <Text>  session=</Text>
        <Text color="yellow">{sessionId || '(new)'}</Text>
        <Text>  state=</Text>
        <Text color={busy ? 'yellow' : 'green'}>{busy ? 'streaming' : 'idle'}</Text>
      </Box>

      <Box>
        <Box flexDirection="column" borderStyle="round" borderColor="cyan" paddingX={1} flexGrow={3} minHeight={20}>
          <Text color="cyan">Chat</Text>
          {visibleMessages.map((m, idx) => (
            <Box key={`${idx}-${m.role}`}>
              <Text color={m.role === 'assistant' ? 'green' : m.role === 'error' ? 'red' : 'white'}>
                {m.role === 'assistant' ? 'TARS' : m.role === 'error' ? 'ERROR' : 'YOU'} {'> '}
              </Text>
              <Text>{m.text}</Text>
            </Box>
          ))}
          {assistantDraft !== '' && (
            <Box>
              <Text color="green">TARS &gt; </Text>
              <Text>{assistantDraft}</Text>
            </Box>
          )}
        </Box>

        <Box width={1} />

        <Box flexDirection="column" borderStyle="round" borderColor="magenta" paddingX={1} flexGrow={2} minHeight={20}>
          <Text color="magenta">Status</Text>
          {visibleStatus.map((line, idx) => (
            <Text key={`status-${idx}`}>• {line}</Text>
          ))}
          {initial.verbose && (
            <>
              <Text color="magentaBright">Debug</Text>
              {visibleDebug.map((line, idx) => (
                <Text key={`debug-${idx}`} dimColor>
                  {line}
                </Text>
              ))}
            </>
          )}
        </Box>
      </Box>

      <Box marginTop={1}>
        <Text color="yellow">You &gt; </Text>
        <TextInput
          value={input}
          onChange={setInput}
          onSubmit={() => {
            void submit();
          }}
          placeholder={busy ? 'waiting for response...' : 'Type message and press Enter'}
          focus={!busy}
        />
      </Box>
    </Box>
  );
}

const options = parseArgs(process.argv.slice(2));
render(<App initial={options} />);

