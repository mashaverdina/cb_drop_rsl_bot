package rslbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vkokarev.com/rslbot/pkg/keyboards"
)

var monthMap = map[string]time.Month{
	keyboards.Jan: time.January,
	keyboards.Feb: time.February,
	keyboards.Mar: time.March,
	keyboards.Apr: time.April,
	keyboards.May: time.May,
	keyboards.Jun: time.June,
	keyboards.Jul: time.July,
	keyboards.Aug: time.August,
	keyboards.Sep: time.September,
	keyboards.Oct: time.October,
	keyboards.Nov: time.November,
	keyboards.Dec: time.December,
}

type ProcessingMessage struct {
	UserID    int64
	ChatID    int64
	MessageID int
	Text      string
}

type Processor interface {
	Handle(ctx context.Context, state UserState, msg *ProcessingMessage) (UserState, tgbotapi.Chattable, error)
}

type MainProcessor struct {
}

func (p *MainProcessor) Handle(ctx context.Context, state UserState, msg *ProcessingMessage) (UserState, tgbotapi.Chattable, error) {
	switch msg.Text {
	case keyboards.Cb5:
		state.State = StateCb5
		resp := tgbotapi.NewMessage(msg.ChatID, "Что упало с 5го КБ?")
		resp.ReplyMarkup = keyboards.AddDropInlineKeyboard
		return state, resp, nil
	case keyboards.Cb6:
		state.State = StateCb6
		resp := tgbotapi.NewMessage(msg.ChatID, "Что упало с 6го КБ?")
		resp.ReplyMarkup = keyboards.AddDropInlineKeyboard
		return state, resp, nil
	case keyboards.Stats:
		state.State = StateStats
		resp := tgbotapi.NewMessage(msg.ChatID, "Что тебе показать?")
		resp.ReplyMarkup = keyboards.StatsKeyboard
		return state, resp, nil
	}

	resp := tgbotapi.NewMessage(msg.ChatID, "Привет")
	resp.ReplyMarkup = keyboards.MainMenuKeyboard
	return state, resp, nil
}

type CbProcessor struct {
	level   int
	stats   map[int64]CbUserState
	storage *CbStatStorage
}

func NewCbProcessor(level int, storage *CbStatStorage) *CbProcessor {
	return &CbProcessor{
		level:   level,
		stats:   make(map[int64]CbUserState),
		storage: storage,
	}
}

func (p *CbProcessor) Handle(ctx context.Context, state UserState, msg *ProcessingMessage) (UserState, tgbotapi.Chattable, error) {
	cbState := p.getOrCreateStats(state.UserID)
	switch msg.Text {
	case keyboards.Reject:
		state.State = StateMainMenu
		resp := tgbotapi.NewMessage(msg.ChatID, "До встречи")
		resp.ReplyMarkup = keyboards.MainMenuKeyboard
		// p.stats[state.UserID] = NewCbUserState(state.UserID)
		delete(p.stats, state.UserID)
		return state, resp, nil
	case keyboards.Approve:
		state.State = StateMainMenu

		cbState := p.stats[state.UserID]
		err := p.storage.Save(ctx, &cbState)
		if err != nil {
			return UserState{}, nil, err
		}

		resp := tgbotapi.NewMessage(msg.ChatID, "Записано")
		resp.ReplyMarkup = keyboards.MainMenuKeyboard
		p.stats[state.UserID] = NewCbUserState(state.UserID, p.level)
		return state, resp, nil
	case keyboards.Clear:
		cbState = NewCbUserState(state.UserID, p.level)
	case keyboards.LegTome:
		p.increment(&cbState.LegTome)
	case keyboards.AncientShard:
		p.increment(&cbState.AncientShard)
	case keyboards.VoidShard:
		p.increment(&cbState.VoidShard)
	case keyboards.SacredShard:
		p.increment(&cbState.SacredShard)
	case keyboards.EpicTome:
		p.increment(&cbState.EpicTome)
	default:
		resp := tgbotapi.NewMessage(msg.ChatID, "АХАХАХХАА ТЫТ ТУТ ЗАВИС (Нажми закрыть)")
		return state, resp, nil
	}

	resp := tgbotapi.NewEditMessageText(msg.ChatID, msg.MessageID, msgFromStat(cbState, p.level))
	resp.ReplyMarkup = &keyboards.AddDropInlineKeyboard
	resp.ParseMode = "markdown"
	p.stats[state.UserID] = cbState
	return state, resp, nil

}

func msgFromStat(state CbUserState, level int) string {
	lines := []string{}
	if level > 0 {
		lines = append(lines, fmt.Sprintf("Стата по *%d КБ*", level))
	}

	lines = append(lines, fmt.Sprintf("%s -- %d", keyboards.AncientShard, state.AncientShard))
	lines = append(lines, fmt.Sprintf("%s -- %d", keyboards.VoidShard, state.VoidShard))
	lines = append(lines, fmt.Sprintf("%s -- %d", keyboards.SacredShard, state.SacredShard))
	lines = append(lines, fmt.Sprintf("%s -- %d", keyboards.EpicTome, state.EpicTome))
	lines = append(lines, fmt.Sprintf("%s -- %d", keyboards.LegTome, state.LegTome))

	return strings.Join(lines, "\n")
}

func (p *CbProcessor) getOrCreateStats(userID int64) CbUserState {
	if s, ok := p.stats[userID]; ok && !s.Expired() {
		return s
	}
	s := NewCbUserState(userID, p.level)
	p.stats[userID] = s
	return s
}

func (p *CbProcessor) increment(val *int) {
	*val = *val + 1
}

type StatsProcessor struct {
	cbStatStorage *CbStatStorage
}

func NewStatsProcessor(cbStatStorage *CbStatStorage) *StatsProcessor {
	return &StatsProcessor{
		cbStatStorage: cbStatStorage,
	}
}

func (p *StatsProcessor) LastStat(ctx context.Context, msg *ProcessingMessage, resource string, header string) (tgbotapi.Chattable, error) {
	lastFrom5, err := p.cbStatStorage.LastResource(ctx, msg.UserID, 5, resource)
	if err != nil {
		return nil, err
	}
	lastFrom6, err := p.cbStatStorage.LastResource(ctx, msg.UserID, 6, resource)
	if err != nil {
		return nil, err
	}

	resp := tgbotapi.NewMessage(msg.ChatID, strings.Join([]string{
		header,
		fmt.Sprintf("С 5го -- %s", timePast(lastFrom5)),
		fmt.Sprintf("С 6го -- %s", timePast(lastFrom6)),
	}, "\n"))
	resp.ReplyMarkup = keyboards.MainMenuKeyboard
	return resp, nil
}

func (p *StatsProcessor) Handle(ctx context.Context, state UserState, msg *ProcessingMessage) (UserState, tgbotapi.Chattable, error) {
	switch msg.Text {
	case keyboards.Back:
		state.State = StateMainMenu
		resp := tgbotapi.NewMessage(msg.ChatID, "До встречи")
		resp.ReplyMarkup = keyboards.MainMenuKeyboard
		return state, resp, nil
	case keyboards.LastVoidShard:
		state.State = StateMainMenu
		resp, err := p.LastStat(ctx, msg, "void_shard", "Последний темный осколок")
		return state, resp, err
	case keyboards.LastSacredShard:
		state.State = StateMainMenu
		resp, err := p.LastStat(ctx, msg, "sacred_shard", "Последний сакральный осколок")
		return state, resp, err
	case keyboards.LastLegTome:
		state.State = StateMainMenu
		resp, err := p.LastStat(ctx, msg, "leg_tome", "Последний лег том")
		return state, resp, err
	case keyboards.MonthStats:
		state.State = StateMonth
		resp := tgbotapi.NewMessage(msg.ChatID, "Выбери месяц")
		resp.ReplyMarkup = keyboards.ChooseMonthKeyboard
		return state, resp, nil
	default:
		resp := tgbotapi.NewMessage(msg.ChatID, "АХАХАХХАА ТЫТ ТУТ ЗАВИС (Нажми закрыть)")
		return state, resp, nil
	}
}

type MonthProcessor struct {
	cbStatStorage *CbStatStorage
}

func NewMonthProcessor(cbStatStorage *CbStatStorage) *MonthProcessor {
	return &MonthProcessor{
		cbStatStorage: cbStatStorage,
	}
}

func (p *MonthProcessor) Handle(ctx context.Context, state UserState, msg *ProcessingMessage) (UserState, tgbotapi.Chattable, error) {
	switch msg.Text {
	case keyboards.Back:
		state.State = StateStats
		resp := tgbotapi.NewMessage(msg.ChatID, "Что тебе показать?")
		resp.ReplyMarkup = keyboards.StatsKeyboard
		return state, resp, nil
	case keyboards.Jan, keyboards.Feb, keyboards.Mar, keyboards.Apr, keyboards.May, keyboards.Jun, keyboards.Jul, keyboards.Aug, keyboards.Sep, keyboards.Oct, keyboards.Nov, keyboards.Dec:
		state.State = StateMainMenu
		// TODO: get stats for month from DB
		from, to := mothInterval(msg.Text)
		monthStat, err := p.cbStatStorage.UserStat(ctx, msg.UserID, []int{5, 6}, from, to)
		var replyMsg = ""
		if err != nil {
			replyMsg = "Статистики пока нет"
		} else {
			statText := msgFromStat(monthStat, 0)
			replyMsg = fmt.Sprintf("Вот твоя статистика за %s:\n%s", msg.Text, statText)
		}

		resp := tgbotapi.NewMessage(msg.ChatID, replyMsg)
		resp.ReplyMarkup = keyboards.MainMenuKeyboard
		return state, resp, nil
	default:
		resp := tgbotapi.NewMessage(msg.ChatID, "АХАХАХХАА ТЫТ ТУТ ЗАВИС (Нажми закрыть)")
		return state, resp, nil
	}
}

func timePast(t *time.Time) string {
	if t == nil {
		return "никогда"
	}
	delta := time.Now().Sub(*t)
	if delta.Hours() < 24 {
		return "сегодня"
	}
	return strconv.FormatInt(int64(delta.Hours()/24), 10) + " д. назад"
}

func mothInterval(month string) (time.Time, time.Time) {
	mn := monthMap[month]
	cy, cm, _ := time.Now().Date()
	if cm < mn {
		cy = cy - 1
	}

	from := time.Date(cy, mn, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, -1)
	return from, to
}
