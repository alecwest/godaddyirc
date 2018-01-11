// Package dad is an extension of hellabot that plays the role of an IRC
// chat bot, either as a mom or a dad
package dad

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/whyrusleeping/hellabot"
	log "gopkg.in/inconshreveable/log15.v2"
)

// Configuration lists all the high-level content of the config file
type Configuration struct {
	Admin       string
	AdminSpeak  []SpeakData
	Channels    []string
	DadName     string
	Debug       bool
	Grounded    []string
	Ip          string
	MessageRate int // Using 1 reply per x seconds
	MomName     string
	MomSpeak    []SpeakData
	Speak       []SpeakData
	Timeout     int // Timeout between multi-lined reply
}

// SpeakData is the regex-to-response pairing for each possible response.
// There can be more than one response, and it will be chosen semi-randomly.
type SpeakData struct {
	Regex    string
	Response []ResponseData
}

// ResponseData contains the bot's reply and the number of times the reply
// has been sent. Message may contain [...] blocks for different types of
// text replacement/manipulation.
type ResponseData struct {
	Message string
	Count   int
}

// Reply includes the final formatted response (all text replacement blocks
// dealt with), the destination, and the time the message was sent at.
type Reply struct {
	Content []string
	To      string
	Sent    time.Time
}

// IRCBot is an extension of hellabot's Bot that includes an indicator for
// whether the bot is acting as mom or dad, the config information, and the
// last reply sent by the bot
type IRCBot struct {
	Bot       *hbot.Bot
	Dad       bool
	Conf      Configuration
	LastReply Reply
}

// Dbot is the global variable that primarily allows for the config information
// to be smoothly passed around and updated properly.
var Dbot IRCBot

// Run starts an instance of the bot, with variable dad indicating whether
// the bot should behave like a dad or a mom
func Run(dad bool) {
	var nickStr string
	rand.Seed(time.Now().Unix())
	flag.Parse()
	Dbot.Conf = InitConfig()
	Dbot.Dad = dad
	if Dbot.Dad {
		nickStr = Dbot.Conf.DadName
	} else {
		nickStr = Dbot.Conf.MomName
	}
	serv := flag.String("server", Dbot.Conf.Ip+
		":6667", "hostname and port for irc server to connect to")
	nick := flag.String("nick", nickStr, "nickname for the bot")

	hijackSession := func(bot *hbot.Bot) {
		bot.HijackSession = false
	}
	channels := func(bot *hbot.Bot) {
		bot.Channels = Dbot.Conf.Channels
	}
	bot, err := hbot.NewBot(*serv, *nick, hijackSession, channels)
	Dbot.Bot = bot
	if err != nil {
		panic(err)
	}
	Dbot.Bot.AddTrigger(UserTrigger)
	Dbot.Bot.AddTrigger(AdminTrigger)
	Dbot.Bot.Logger.SetHandler(log.StdoutHandler)
	// Start up bot (this blocks until we disconnect)
	Dbot.Bot.Run()
	fmt.Println("Bot shutting down.")
}

// InitConfig returns an initialized config.
func InitConfig() Configuration {
	file, _ := os.Open("conf.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	conf := Configuration{}
	err := decoder.Decode(&conf)
	if err != nil {
		panic(err)
	}
	return conf
}

// UpdateConfig parses the current config information and rewrites it to
// the config file.
func UpdateConfig() {
	jsonData, err := json.MarshalIndent(Dbot.Conf, "", "    ")
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile("conf.json", jsonData, 0644)
}

// Ground checks the list of currently grounded users and adds the name if
// it has not yet been added.
func Ground(name string) {
	i := StringInSlice(name, Dbot.Conf.Grounded)
	if i != -1 { return }
	Dbot.Conf.Grounded = append(Dbot.Conf.Grounded, name)
}

// Unground checks the list of grounded users for the requested name and
// removes it if it is found.
func Unground(name string) {
	i := StringInSlice(name, Dbot.Conf.Grounded)
	if i == -1 { return }
	Dbot.Conf.Grounded[len(Dbot.Conf.Grounded) - 1], Dbot.Conf.Grounded[i] =
		Dbot.Conf.Grounded[i], Dbot.Conf.Grounded[len(Dbot.Conf.Grounded) - 1]
	Dbot.Conf.Grounded = Dbot.Conf.Grounded[:len(Dbot.Conf.Grounded) - 1]
}

// TestMessage tests the passed message against the passed regex and returns
// whether or not a match was found
func TestMessage(regex string, message *hbot.Message) bool {
	match := false
	// err = errors.New("Forgot to include who the message was from")
	r := regexp.MustCompile(regex)
	if r.MatchString(message.Content) {
		match = true
	}
	return match
}

// StripWhitespace removes all whitespace from the given string and returns it
func StripWhitespace(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, str)
}

// MessageRateMet checks whether or not enough time has passed since the Last
// reply was sent. If the message just sent was from an admin, ignore
// time passed.
func MessageRateMet(message *hbot.Message) bool {
	return (time.Since(Dbot.LastReply.Sent) > (time.Duration(Dbot.Conf.MessageRate)*time.Second) || message.From == Dbot.Conf.Admin)
}

// StringInSlice checks slice s for string a and returns the first matching
// index, and -1 otherwise
func StringInSlice(a string, s []string) int {
	for i, b := range s {
		if a == b {
			return i
		}
	}
	return -1
}

// RemoveRegex removes the substring matching the passed regex from the passed
// string, s, and returns the result.
func RemoveRegex(s string, regex string) string {
	r := regexp.MustCompile(regex)
	return r.ReplaceAllLiteralString(s, "")
}

// SetRecipient modifies m's Content to no longer contain the regex command
// match in s as well as removes the recipient of the bot's reply (formatted
// as <cmd> <recipient>: <rest>). It returns the recipient, or the bot's
// primary channel if a recipient was not specified.
func SetRecipient(m *hbot.Message, s SpeakData) string {
	to := ""
	strWithoutCommand := RemoveRegex(m.Content, s.Regex)
	// log.Debug(strWithoutCommand)
	to = RemoveRegex(strWithoutCommand, ":.*")
	if to == strWithoutCommand {
		to = Dbot.Conf.Channels[0]
	}
	m.Content = strWithoutCommand
	m.Content = RemoveRegex(m.Content, ".*:\\s")
	return to
}

// FormatReply formulates the bot's response given the message, whether or
// not the sender was an admin (admin_speak), and the index of the SpeakData
// to format the reply to (s_index). It returns the reply with set content and
// destination (but not the time).
func FormatReply(message *hbot.Message, admin_speak bool, s_index int) Reply {
	s := getSpeakData(admin_speak)[s_index]
	var reply Reply
	// Choose random response from list of responses (mostly used for jokes)
	var rand_index = rand.Intn(len(s.Response))
	rand_index = GetRandomLeastUsedResponseIndex(s)
	response := s.Response[rand_index]
	// Stolen from Bot.Reply to init reply.To
	if strings.Contains(message.To, "#") {
		reply.To = message.To
	} else {
		reply.To = message.From
	}
	if strings.Contains(response.Message, "[from]") {
		response.Message = strings.Replace(response.Message, "[from]", message.From, -1)
	}
	if strings.Contains(response.Message, "[grounded]") {
		response.Message = strings.Replace(response.Message, "[grounded]",
			strings.Join(Dbot.Conf.Grounded, ", "),
			-1)
	}
	// Manages all responses that reuse any content from the original message
	for _, replace := range []string{"[mock]", "[repeat]", "[ground]",
		"[unground]", "[poof]"} {
		if strings.Contains(response.Message, replace) {
			// Modify who the message is sent to if it includes "user:" before the cmd
			if replace == "[repeat]" {
				to := SetRecipient(message, s)
				if len(to) > 0 {
					reply.To = to
				}
			} else {
				// Remove the part that the regex matched to
				message.Content = StripWhitespace(RemoveRegex(message.Content, s.Regex))
			}
			if replace == "[ground]" {
				Ground(message.Content)
			} else if replace == "[unground]" {
				Unground(message.Content)
			} else if replace == "[poof]" {
				message.Content = AddArticle(message.Content)
			}
			// Replace [...] element with what remains in the Content of the message
			nonWord := regexp.MustCompile("^\\W+$")
			if len(message.Content) == 0 || nonWord.MatchString(message.Content) {
				response.Message = "" // Delete response if m.Content is empty
			} else {
				response.Message = strings.Replace(response.Message, replace,
					message.Content, -1)
			}
		}
	}
	// If message is non-empty, then bot will send it, so increment response count
	if response.Message != "" {
		s.Response[rand_index].Count++
	}
	reply.Content = strings.Split(response.Message, "\n")
	return reply
}

// PerformAction determines whether or not a reply should be formulated and then
// performs it by passing it the bot in use (irc), the message just sent (m),
// and whether or not the sender was the admin (admin_speak). If an action
// was performed, return true.
func PerformAction(irc *hbot.Bot, m *hbot.Message, admin_speak bool) bool {
	speak := getSpeakData(admin_speak)
	// Do not perform an action if either the sender is grounded, is mom/dad,
	// sufficient time has not passed, or the message is from the irc's IP
	if StringInSlice(m.From, Dbot.Conf.Grounded) != -1 ||
	  StringInSlice(m.From, []string{Dbot.Conf.MomName, Dbot.Conf.DadName}) != -1 ||
		MessageRateMet(m) == false ||
		StringInSlice(m.From, []string{Dbot.Conf.Ip, "irc.awest.com"}) != -1 {
		return false
	}
	for i, s := range speak {
		if TestMessage(s.Regex, m) {
			reply := FormatReply(m, admin_speak, i)
			reply.Sent = time.Now()
			numSent := 0
			for _, line := range reply.Content {
				// Make sure line is non-empty before sending
				if len(line) > 0 {
					irc.Msg(reply.To, fmt.Sprintf(line))
					numSent++
				}
				// Make sure there is a timeout between multiple lines in a reply
				if len(reply.Content) > 1 && numSent > 0 {
					time.Sleep(time.Duration(Dbot.Conf.Timeout) * time.Second)
				}
			}
			if numSent > 0 {
				// Record last sent message
				Dbot.LastReply = reply
				UpdateConfig()
				return true
			}
			// If a regex statement passed but nothing was sent,
			// the loop should not continue trying to match the reply to others.
			break
		}
	}
	return false
}

// GetRandomLeastUsedResponseIndex chooses a random response among all
// within the given speak data, giving priority to responses that have not
// yet been used as much. It returns the index of the response it chose
func GetRandomLeastUsedResponseIndex(speak SpeakData) int {
	var minCount = math.MaxUint32
	chosenIndex := rand.Intn(len(speak.Response))
	for _, response := range speak.Response {
		if response.Count < minCount {
			minCount = response.Count
		}
	}
	for speak.Response[chosenIndex].Count > minCount {
		chosenIndex = rand.Intn(len(speak.Response))
	}
	log.Debug(fmt.Sprintf("Chosen response %d : %s", chosenIndex, speak.Response[chosenIndex].Message))
	return chosenIndex
}

// Prepand the given string with an "a" or "an" based on the first word and
// return the result
func AddArticle(s string) string {
	for _, vowel := range []string{"a", "e", "i", "o", "u"} {
		if strings.Contains(vowel, string(s[0])) {
			return "an " + s
		}
	}
	return "a " + s
}

func getSpeakData(admin_speak bool) []SpeakData {
	var s []SpeakData
	if Dbot.Dad == false {
		s = Dbot.Conf.MomSpeak
	} else if admin_speak {
		s = Dbot.Conf.AdminSpeak
	} else {
		s = Dbot.Conf.Speak
	}
	return s;
}

// UserTrigger is for all non-admin users.
var UserTrigger = hbot.Trigger{
	func(bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From != Dbot.Conf.Admin)
	},
	func(irc *hbot.Bot, m *hbot.Message) bool {
		PerformAction(irc, m, false)
		UpdateConfig()
		return false
	},
}

// AdminTrigger is for the admin user. If no admin response is performed,
// a user reponse is attempted.
var AdminTrigger = hbot.Trigger{
	func(bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From == Dbot.Conf.Admin)
	},
	func(irc *hbot.Bot, m *hbot.Message) bool {
		responded := PerformAction(irc, m, true)
		if !responded {
			PerformAction(irc, m, false)
		}
		UpdateConfig()
		return false
	},
}
