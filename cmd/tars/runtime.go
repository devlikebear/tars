package main

import "net/http"

type runtimeClient struct {
	serverURL     string
	apiToken      string
	adminAPIToken string
	httpClient    *http.Client
}
