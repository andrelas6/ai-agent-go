package messages

type Service struct {
	// deps
	messages       []string
	messagesStream chan string
}

func NewService() *Service {
	return &Service{
		messages:       make([]string, 0),
		messagesStream: make(chan string),
	}
}

func (c *Service) AddMessage(message string) {
	c.messages = append(c.messages, message)
	c.messagesStream <- message
}

func (c *Service) MessagesStream() <-chan string {
	return c.messagesStream
}
