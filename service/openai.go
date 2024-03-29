package service

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/sashabaranov/go-openai"
	log "github.com/sirupsen/logrus"
)

type OpenAIConfig struct {
	Enable   bool   `yaml:"Enable"`
	Endpoint string `yaml:"Endpoint"`
	Token    string `yaml:"Token"`
}

type OpenAIService struct {
	*OpenAIConfig
	client    *openai.Client
	clientCtx context.Context
	talks     []*OpenAIChatGPTTalk
}

type OpenAIChatGPTTalk struct {
	Messages []struct {
		IsUser    bool
		MessageID int
		Message   string
	}
	LastUsedAt int64
}

var OpenAIInstance = NewOpenAIService()

func NewOpenAIService() *OpenAIService {
	return &OpenAIService{
		OpenAIConfig: nil,
		client:       nil,
		clientCtx:    nil,
	}
}

func (service *OpenAIService) Init(conf *OpenAIConfig) {
	service.OpenAIConfig = conf
	service.client = openai.NewClient(service.OpenAIConfig.Token)
	service.clientCtx = context.Background()
}

func (service *OpenAIService) AddTalk(talk *OpenAIChatGPTTalk) {
	service.talks = append(service.talks, talk)
}

func (service *OpenAIService) GetTalkByMessageID(messageId int) *OpenAIChatGPTTalk {
	for _, t := range service.talks {
		for _, m := range t.Messages {
			if m.MessageID == messageId {
				return t
			}
		}
	}
	return nil
}

func (service *OpenAIService) ChatCompletion(messages []openai.ChatCompletionMessage, onResp func(string),
	onFail func(error), retry int) {

	if retry > 0 {
		if retry <= 5 {
			onFail(errors.New("遇到API错误，正在重试。重试次数：" + strconv.Itoa(retry)))
		} else {
			onFail(errors.New("失败重试次数过多，请稍后重试或联系管理员检查Log"))
			return
		}
	}

	resp, err := service.client.CreateChatCompletion(
		service.clientCtx,
		openai.ChatCompletionRequest{
			Model:    openai.GPT3Dot5Turbo,
			Messages: messages,
		},
	)

	e := &openai.APIError{}
	if errors.As(err, &e) {
		log.Errorf("调用CreateChatCompletion时遇到API错误：%s", e.Code)
		service.ChatCompletion(messages, onResp, onFail, retry+1)
		return
	}

	onResp(resp.Choices[0].Message.Content)
}

func (service *OpenAIService) ChatStreamCompletion(messages []openai.ChatCompletionMessage, onResp func(string, bool),
	onFail func(error), retry int) {

	if retry > 0 {
		if retry <= 5 {
			onFail(errors.New("遇到API错误，正在重试。重试次数：" + strconv.Itoa(retry)))
		} else {
			onFail(errors.New("失败重试次数过多，请稍后重试或联系管理员检查Log"))
			return
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    openai.GPT3Dot5Turbo,
		Messages: messages,
		Stream:   true,
	}
	stream, err := service.client.CreateChatCompletionStream(service.clientCtx, req)
	e := &openai.APIError{}
	if errors.As(err, &e) {
		log.Errorf("调用CreateChatCompletionStream时遇到API错误：%s", e.Code)
		if stream != nil {
			stream.Close()
		}
		service.ChatStreamCompletion(messages, onResp, onFail, retry+1)
		return
	}
	defer stream.Close()

	startTime := time.Now().Unix()
	stackResp := ""
	finished := false
	for {
		response, err := stream.Recv()

		if err != nil {
			if errors.As(err, &e) {
				log.Errorf("调用stream.Recv时遇到API错误：%s", e.Code)
				service.ChatStreamCompletion(messages, onResp, onFail, retry+1)
				return
			}

			if errors.Is(err, io.EOF) {
				finished = true
			}
		}

		if finished {
			onResp(stackResp, true)
			break
		} else {
			stackResp += response.Choices[0].Delta.Content
		}

		if time.Now().Unix()-startTime >= 3 && stackResp != "" {
			startTime = time.Now().Unix()
			onResp(stackResp, finished)
			stackResp = ""
		}
	}
}

func (service *OpenAIService) GenerateChatCompletionMessage(messages []struct {
	IsUser  bool
	Message string
}) ([]openai.ChatCompletionMessage, error) {
	var chatCompletionMessages []openai.ChatCompletionMessage
	for _, msg := range messages {
		if msg.IsUser {
			chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: msg.Message,
			})
		} else {
			chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msg.Message,
			})
		}

	}
	return chatCompletionMessages, nil
}
