package tarsclient

import "github.com/devlikebear/tars/pkg/tarsclient"

type agentDescriptor = tarsclient.AgentDescriptor

type agentRun = tarsclient.AgentRun

type agentRuntimeStatus = tarsclient.AgentRuntimeStatus

type agentRuntimeReportSummary = tarsclient.AgentRuntimeReportSummary

type agentRuntimeReportRuns = tarsclient.AgentRuntimeReportRuns

type channelReportMessage = tarsclient.ChannelReportMessage

type agentRuntimeReportChannels = tarsclient.AgentRuntimeReportChannels

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
