package service

import (
	"fmt"
	"time"

	"github.com/taraslis453/territory-service-bot/config"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/pkg/logging"
	"github.com/taraslis453/territory-service-bot/pkg/sheets"
	tb "gopkg.in/telebot.v3"
)

// Services stores all service layer interfaces
type Services struct {
	Bot BotService
}

// Options provides options for creating a new service instance via New.
type Options struct {
	Cfg      *config.Config
	Logger   logging.Logger
	Storages Storages
	Sheets   sheets.Service
}

// serviceContext provides a shared context for all services
type serviceContext struct {
	cfg      *config.Config
	logger   logging.Logger
	storages Storages
	sheets   sheets.Service
}

type BotService interface {
	HandleStart(c tb.Context, b *tb.Bot) error
	HandleMessage(c tb.Context, b *tb.Bot) error
	RenderMenu(c tb.Context, b *tb.Bot) error
	HandleInlineButton(c tb.Context, b *tb.Bot) error
	HandleImageUpload(c tb.Context, b *tb.Bot) error
	HandleDocumentUpload(c tb.Context, b *tb.Bot) error
}

var (
	MessageEnterFullName               = "Як мені тебе запам'ятати? (ім’я та фамілія) ✍️"
	MessageEnterCongregationName       = "З якого ти збору? ✍️"
	MessageUserNotFound                = "Ти не зареєстрований в системі. Звернись до адміністратора збору 📞"
	MessageCongregationNotFound        = "Збір не знайдено 🤷"
	MessageCongregationAdminNotFound   = "Адміністраторa збору не знайдено 🤷"
	MessageUserIsNotAdmin              = "Ти не є адміністратором збору 🤷"
	MessageCongregationJoinRequestSent = func(congregationName string) string {
		return fmt.Sprintf("Запит на приєднання до збору *%s* відправлено. Очікуй відповідь 😌", congregationName)
	}
	MessageWaitingForAdminApproval = "Очікуй підтвердження адміністратора збору 😌"
	MessageNewJoinRequest          = func(options *MessageNewJoinRequestOptions) string {
		userFullName := fmt.Sprintf("%s %s", options.FirstName, options.LastName)
		if options.Username != "" {
			userFullName += fmt.Sprintf(" (@%s)", options.Username)
		}
		message := fmt.Sprint(userFullName, " хоче приєднатися")
		return message
	}
	MessageCongregationJoinRequestApprovedDone = func(fullName string) string {
		return fmt.Sprintf("Вісника *%s* приєднано до збору ✅", fullName)
	}
	MessageCongregationJoinRequestRejectedDone = func(fullName string) string {
		return fmt.Sprintf("Користувача *%s* відхилено ❌", fullName)
	}
	MessageCongregationJoinRequestApproved = "Запит на приєднання до збору прийнято 🎉"
	MessageCongregationJoinRequestRejected = "Запит на приєднання до збору відхилено 😔"

	MessageHowCanIHelpYou          = "Чим можу допомогти? 🙂"
	MessageAddTerritoryInstruction = "Надішли зображення або документ території де повідомлення відповідає зразку: *Група_назва* \nНаприклад: *Львів_123-а*, *Рівне_200* 📸"
	MessageTerritoryExistsInGroup  = func(title string, groupTitle string) string {
		return fmt.Sprintf("Територія з назвою *%s* вже існує в групі *%s* 🤷", title, groupTitle)
	}
	MessageNoTerritoriesFound              = "Території не знайдено 🤷"
	MessageTerritoryNotFound               = "Територія не знайдена 🤷"
	MessageTerritoryNotAvailable           = "Територія не доступна 🤷"
	MessageTerritoryList                   = "Список доступних територій: "
	MessageMyTerritoryListTerritoryCaption = func(title string, lastTakenAt time.Time, notes []string) string {
		caption := fmt.Sprintf("Територія: %s\n%s", title, lastTakenAt.Format("02.01.2006"))
		if len(notes) > 0 {
			caption += "\n\n"
			caption += "Нотатки:\n"
			for _, note := range notes {
				caption += fmt.Sprintf("📌 %s\n", note)
			}
		}
		return caption
	}
	MessageTerritoryListTerritoryCaption = func(options MessageTerritoryListTerritoryCaptionOptions) string {
		caption := fmt.Sprintf("Територія: %s", options.Title)
		if !options.LastTakenAt.IsZero() {
			caption += fmt.Sprintf("\nОстаннє опрацювання: *%s*", options.LastTakenAt.Format("02.01.2006"))
		}

		if options.UserRole == entity.UserRoleAdmin {
			if options.InUseByFullName != "" {
				caption += fmt.Sprintf("\nВикористовує: *%s*", options.InUseByFullName)
			}

			if len(options.Notes) > 0 {
				caption += "\n\n"
				caption += "Нотатки:\n"
				for _, note := range options.Notes {
					caption += fmt.Sprintf("📌 %s\n", note)
				}
			}
		}
		return caption
	}

	MessageTerritoryTaken = func(territoryTitle string, notes []string) string {
		message := fmt.Sprintf("Територію *%s* взято ✅", territoryTitle)
		if len(notes) > 0 {
			message += "\n\n"
			message += "Нотатки:\n"
			for _, note := range notes {
				message += fmt.Sprintf("📌 %s\n", note)
			}
		}
		return message
	}
	MessagePublisherTookTerritory = func(fullName string, territoryTitle string) string {
		return fmt.Sprintf("Вісник *%s* взяв територію *%s*", fullName, territoryTitle)
	}
	MessagePublisherReturnedTerritory = func(fullName string, territoryTitle string) string {
		return fmt.Sprintf("Вісник *%s* повернув територію *%s* ✅", fullName, territoryTitle)
	}
	MessageLeaveTerritoryNote = func(territoryTitle string) string {
		return fmt.Sprintf("Залишіть нотатку для території %s ✍️", territoryTitle)
	}
	MessageTerritoryNotInUse        = "Територія не використовується 🤷"
	MessageTerritoryCannotLeaveNote = "Ви не можете залишити нотатку для цієї території 🤷"
	MessageTerritoryNoteSaved       = "Нотатку збережено ✅"

	MessageTerritoryReturned = "Територію повернуто ✅"

	MessagePublisherNotFound = "Вісника не знайдено 🤷"

	MessageReportNotConfigured = "Звіт не налаштовано. Зверніться до адміністратора системи для налаштування Google Sheets 📊"
	MessageReportLink          = func(link string) string {
		return fmt.Sprintf("📊 *Звіт по територіях*\n\n[Відкрити таблицю](%s)", link)
	}
)

type MessageNewJoinRequestOptions struct {
	FirstName string
	LastName  string
	Username  string
}

type MessageTerritoryListTerritoryCaptionOptions struct {
	UserRole        entity.UserRole
	Title           string
	LastTakenAt     time.Time
	Notes           []string
	InUseByFullName string
}
