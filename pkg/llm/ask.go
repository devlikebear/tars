package llm

import "context"

func askFromSinglePrompt(
	ctx context.Context,
	chat func(context.Context, []ChatMessage, ChatOptions) (ChatResponse, error),
	prompt string,
	normalizers ...func(string) string,
) (string, error) {
	resp, err := chat(ctx, []ChatMessage{{Role: "user", Content: prompt}}, ChatOptions{})
	if err != nil {
		return "", err
	}
	content := resp.Message.Content
	if len(normalizers) > 0 && normalizers[0] != nil {
		content = normalizers[0](content)
	}
	return content, nil
}
