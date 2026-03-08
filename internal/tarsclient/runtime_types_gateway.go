package tarsclient

import "github.com/devlikebear/tars/pkg/tarsclient"

type agentDescriptor = tarsclient.AgentDescriptor

type agentRun = tarsclient.AgentRun

type gatewayStatus = tarsclient.GatewayStatus

type gatewayReportSummary = tarsclient.GatewayReportSummary

type gatewayReportRuns = tarsclient.GatewayReportRuns

type channelReportMessage = tarsclient.ChannelReportMessage

type gatewayReportChannels = tarsclient.GatewayReportChannels

type browserState = tarsclient.BrowserState

type browserProfile = tarsclient.BrowserProfile

type browserLoginResult = tarsclient.BrowserLoginResult

type browserCheckResult = tarsclient.BrowserCheckResult

type browserRunResult = tarsclient.BrowserRunResult

type vaultStatusInfo = tarsclient.VaultStatusInfo

type telegramPairingPending = tarsclient.TelegramPairingPending

type telegramPairingAllowed = tarsclient.TelegramPairingAllowed

type telegramPairingsInfo = tarsclient.TelegramPairingsInfo

type agentSpawnRequest = tarsclient.SpawnRequest

type spawnCommand struct {
	SessionID string
	Title     string
	Agent     string
	Wait      bool
	Message   string
}
