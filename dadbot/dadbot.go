package dad

// TODO refactor stuff that manages the config (make a config navigation struct or something)
// TODO add attribute for responses that involve reuse (ReuseContent bool)
// TODO replace [...] blocks with %s and put them in a separate attribute (Format string)

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/whyrusleeping/hellabot"
	log "gopkg.in/inconshreveable/log15.v2"
)

type Configuration struct {
	Admin       string
	AdminSpeak  []SpeakData
	Channels    []string
	DadName     string
	Debug       bool
	Grounded    []string
	Ip          string
	MessageRate int // Using 1 reply per x seconds instead of y per x seconds
	MomName     string
	MomSpeak    []SpeakData
	Speak       []SpeakData
	Timeout     int // Timeout between multi-lined reply
}

type SpeakData struct {
	Regex    string
	Response []ResponseData
}

type ResponseData struct {
	Message string
	Count   int
}

type Reply struct {
	Content []string
	To      string
	Sent    time.Time
}

type IRCBot struct {
	Bot       *hbot.Bot
	Dad       bool
	Conf      Configuration
	LastReply Reply
}

var dbot IRCBot

// dad indicates whether the bot being run is a dad-bot or a mom-bot
// starts up an instance of the bot
func Run(dad bool) {
	var nickStr string
	rand.Seed(time.Now().Unix())
	flag.Parse()
	dbot.Conf = InitConfig()
	dbot.Dad = dad
	if dbot.Dad {
		nickStr = dbot.Conf.DadName
	} else {
		nickStr = dbot.Conf.MomName
	}
	serv := flag.String("server", dbot.Conf.Ip+
		":6667", "hostname and port for irc server to connect to")
	nick := flag.String("nick", nickStr, "nickname for the bot")

	hijackSession := func(bot *hbot.Bot) {
		bot.HijackSession = false
	}
	channels := func(bot *hbot.Bot) {
		bot.Channels = dbot.Conf.Channels
	}
	bot, err := hbot.NewBot(*serv, *nick, hijackSession, channels)
	dbot.Bot = bot
	if err != nil {
		panic(err)
	}

	dbot.Bot.AddTrigger(UserTrigger)
	dbot.Bot.AddTrigger(AdminTrigger)
	dbot.Bot.Logger.SetHandler(log.StdoutHandler)

	// Start up bot (this blocks until we disconnect)
	dbot.Bot.Run()
	fmt.Println("Bot shutting down.")
}

// It returns an initialized config
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

// It parses the current config information and rewrites it to the config file
func UpdateConfig() {
	jsonData, err := json.MarshalIndent(dbot.Conf, "", "    ")
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile("conf.json", jsonData, 0644)
}

// content should be a user name that is to be grounded or ungrounded
// command specifies whether the action being performed is ground or unground
// passing anything other than "[ground]" or "[unground]" will do nothing
func UpdateGrounding(content string, command string) {
	i := StringInSlice(content, dbot.Conf.Grounded)

	// log.Debug(fmt.Sprintf("index: %d, grounding/ungrounding: %s", i, content))
	if command == "[ground]" && i == -1 {
		dbot.Conf.Grounded = append(dbot.Conf.Grounded, content)
	} else if command == "[unground]" && i != -1 {
		dbot.Conf.Grounded[len(dbot.Conf.Grounded)-1],
			dbot.Conf.Grounded[i] = dbot.Conf.Grounded[i],
			dbot.Conf.Grounded[len(dbot.Conf.Grounded)-1]
		dbot.Conf.Grounded = dbot.Conf.Grounded[:len(dbot.Conf.Grounded)-1]
	}
}

// regex contains the regex statement to test on the message's content
// message contains the message that was sent
// It returns true if the message matches the regex
func TestMessage(regex string, message *hbot.Message) bool {
	match := false
	// err = errors.New("Forgot to include who the message was from")
	r := regexp.MustCompile(regex)
	if r.MatchString(message.Content) {
		match = true
	}
	return match
}

// m contains the message that was sent
// It returns true if enough time has passed since the last reply was given or if the message was from the bot's admin
func MessageRateMet(m *hbot.Message) bool {
	return (time.Since(dbot.LastReply.Sent) > (time.Duration(dbot.Conf.MessageRate)*time.Second) || m.From == dbot.Conf.Admin)
}

// a contains string to check for
// s contains slice to check in
// It returns the index the string was found in, and -1 otherwise
func StringInSlice(a string, s []string) int {
	for i, b := range s {
		if a == b {
			return i
		}
	}
	return -1
}

// s contains the string to work with
// regex contains the regex for what to remove from s
// It returns the new string
func RemoveRegex(s string, regex string) string {
	r := regexp.MustCompile(regex)
	return r.ReplaceAllLiteralString(s, "")
}

// m contains the message received
// s contains the SpeakData for removing the command portion from the content
// It modifies m's Content to no longer contain the command or recipient
// It returns the targeted recipient, or the primary channel if not specified
func SetRecipient(m *hbot.Message, s SpeakData) string {
	to := ""
	strWithoutCommand := RemoveRegex(m.Content, s.Regex)
	log.Debug(strWithoutCommand)
	to = RemoveRegex(strWithoutCommand, ":.*")
	if to == strWithoutCommand {
		to = dbot.Conf.Channels[0]
	}
	m.Content = strWithoutCommand
	m.Content = RemoveRegex(m.Content, ".*:\\s")
	return to
}

// m contains the message received
// admin_speak indicates where to pull the response from (false = non admin)
// s_index indicates the index within admin/non-admin responses to use
// It returns a Reply with content and destination filled out (not time sent)
func FormatReply(m *hbot.Message, admin_speak bool, s_index int) Reply {
	var s SpeakData
	var reply Reply
	if dbot.Dad == false {
		s = dbot.Conf.MomSpeak[s_index]
	} else if admin_speak {
		s = dbot.Conf.AdminSpeak[s_index]
	} else {
		s = dbot.Conf.Speak[s_index]
	}

	// Choose random response from list of responses (mostly used for jokes)
	var rand_index = rand.Intn(len(s.Response))
	response := s.Response[rand_index]

	// Stolen from Bot.Reply to init reply.To
	if strings.Contains(m.To, "#") {
		reply.To = m.To
	} else {
		reply.To = m.From
	}

	if strings.Contains(response.Message, "[from]") {
		response.Message = strings.Replace(response.Message, "[from]", m.From, -1)
	}
	if strings.Contains(response.Message, "[grounded]") {
		response.Message = strings.Replace(response.Message, "[grounded]",
			strings.Join(dbot.Conf.Grounded, ", "),
			-1)
	}

	// Manages all responses that reuse any content from the original message
	for _, replace := range []string{"[mock]", "[repeat]", "[ground]",
		"[unground]"} {
		if strings.Contains(response.Message, replace) {
			// Modify who the message is sent to if it includes "user:" before the cmd
			if replace == "[repeat]" {
				to := SetRecipient(m, s)
				log.Debug(fmt.Sprintf("TESTING:: to: %s", to))
				if len(to) > 0 {
					reply.To = to
				}
			} else {
				// Remove the part that the regex matched to
				m.Content = RemoveRegex(m.Content, s.Regex)
			}

			// Manage grounding/ungrounding
			UpdateGrounding(m.Content, replace)

			// Replace [...] element with what remains in the Content of the message
			nonWord := regexp.MustCompile("^\\W+$")
			if len(m.Content) == 0 || nonWord.MatchString(m.Content) {
				response.Message = "" // Delete response if m.Content is empty
			} else {
				response.Message = strings.Replace(response.Message, replace,
					m.Content, -1)
			}
		}
	}
	if response.Message != "" {
		if dbot.Dad == false {
			dbot.Conf.MomSpeak[s_index].Response[rand_index].Count++
		} else if admin_speak {
			dbot.Conf.AdminSpeak[s_index].Response[rand_index].Count++
		} else {
			dbot.Conf.Speak[s_index].Response[rand_index].Count++
		}
	}
	reply.Content = strings.Split(response.Message, "\n")
	return reply
}

// irc is the bot
// m contains the message that was sent
// admin_speak indicates where to pull the response from (false = non admin)
// It returns true if a reply is needed and false otherwise
func PerformAction(irc *hbot.Bot, m *hbot.Message, admin_speak bool) bool {
	var speak []SpeakData
	if dbot.Dad == false {
		speak = dbot.Conf.MomSpeak
	} else if admin_speak {
		speak = dbot.Conf.AdminSpeak
	} else {
		speak = dbot.Conf.Speak
	}
	// Do not perform an action if either the sender is grounded, sufficient time
	// has not passed, or the message is from the irc's IP
	if StringInSlice(m.From, dbot.Conf.Grounded) != -1 ||
		MessageRateMet(m) == false ||
		StringInSlice(m.From, []string{dbot.Conf.Ip, "irc.awest.com"}) != -1 {
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
					time.Sleep(time.Duration(dbot.Conf.Timeout) * time.Second)
				}
			}
			if numSent > 0 {
				// Record last sent message
				dbot.LastReply = reply
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

// Trigger for all non-admin users
var UserTrigger = hbot.Trigger{
	func(bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From != dbot.Conf.Admin)
	},
	func(irc *hbot.Bot, m *hbot.Message) bool {
		PerformAction(irc, m, false)
		UpdateConfig()
		return false
	},
}

// Trigger for admin user.
// If no admin response is found, a user reponse is attempted
var AdminTrigger = hbot.Trigger{
	func(bot *hbot.Bot, m *hbot.Message) bool {
		return (m.From == dbot.Conf.Admin)
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
