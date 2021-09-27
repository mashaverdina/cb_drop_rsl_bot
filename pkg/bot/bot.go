package rslbot

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	ctx     context.Context
	botAPI  *tgbotapi.BotAPI
	states  map[int]UserState
	cancel  context.CancelFunc
	updates tgbotapi.UpdatesChannel
	done    chan interface{}
	m       sync.Mutex
	started bool
	stopped bool
}

func NewBot(botAPI *tgbotapi.BotAPI) *Bot {
	return &Bot{
		ctx:     nil,
		botAPI:  botAPI,
		states:  make(map[int]UserState),
		cancel:  nil,
		updates: nil,
		done:    make(chan interface{}),
		m:       sync.Mutex{},
		started: false,
		stopped: false,
	}
}

func (b *Bot) Start(ctx context.Context) error {
	b.m.Lock()
	defer b.m.Unlock()

	if b.started {
		return errors.New("started already")
	}

	b.ctx, b.cancel = context.WithCancel(ctx)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.botAPI.GetUpdatesChan(u)
	go b.loop(updates)

	b.started = true

	return nil
}

func (b *Bot) Stop(timeout time.Duration) error {
	b.m.Lock()
	defer b.m.Unlock()

	if !b.started || b.stopped {
		return errors.New("invalid state")
	}

	b.cancel()
	select {
	case <-b.done:
		return nil
	case <-time.After(timeout):
		return errors.New("can't close bot due to timeout")
	}
}

func (b *Bot) loop(updates tgbotapi.UpdatesChannel) {
	log.Println("starting bot loop")
	for update := range updates {
		select {
		case <-b.ctx.Done():
			b.done <- true
			return
		default:
		}

		if update.Message != nil {
			// Construct a new message from the given chat ID and containing
			// the text that we received.
			// msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

			// If the message was open, add a copy of our numeric keyboard.
			// switch update.Message.Text {
			// case "/start", "start":
			// 	msg.Text = "Выбирай действие"
			// 	msg.ReplyMarkup = keyboards.HelloKeyboard
			// // case "":
			// // 	msg.ReplyMarkup = keyboards.NumericKeyboard
			// case "/stop", "stop":
			// 	msg.Text = "Пока"
			// 	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
			// default:
			msg, err := b.processUserMessage(update.Message)
			if err != nil {
				log.Fatalf("got error, while processing message: %v", err)
			}

			// Send the message.
			if _, err := b.botAPI.Send(msg); err != nil {
				// todo не паниковать
				panic(err)
			}
		} else if update.CallbackQuery != nil {
			// Respond to the callback query, telling Telegram to show the user
			// a message with the data received.
			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
			if _, err := b.botAPI.Request(callback); err != nil {
				panic(err)
			}

			// And finally, send a message containing the data received.
			msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data)
			if _, err := b.botAPI.Send(msg); err != nil {
				panic(err)
			}
		}
	}
	log.Println("exiting loop")
}

func (b *Bot) getOrCreateCBState(userID int) UserState {
	if s, ok := b.states[userID]; ok {
		return s
	}
	s := NewUserState(userID)
	b.states[userID] = s
	return s
}

func (b *Bot) processUserMessage(msg *tgbotapi.Message) (tgbotapi.Chattable, error) {
	userID := msg.From.ID
	state := b.getOrCreateCBState(userID)

	processor, err := b.findProcessor(state.State)
	if err != nil {
		return nil, err
	}
	newState, response, err := processor.Handle(state, msg)
	if err != nil {
		return nil, err
	}
	b.states[userID] = newState
	return response, nil
}

func (b *Bot) findProcessor(state State) (Processor, error) {
	switch state {
	case MainMenu:
		return &MainProcessor{}, nil
	case Cb5:
		return &CbProcessor{Level: 5}, nil
	case Cb6:
		return &CbProcessor{Level: 6}, nil
	default:
		return nil, errors.New("not found")
	}
}
