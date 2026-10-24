package bot

// Emoji groups appended to replies. One emoji is picked uniformly at random
// from the group each time.
var (
	// EmojiCelebrate follows a correct quiz answer.
	EmojiCelebrate = []string{"\U0001F60A", "\U0001F60D", "\U0001F31F", "✨", "\U0001F389", "\U0001F38A"}
	// EmojiRegret follows a wrong quiz answer.
	EmojiRegret = []string{"\U0001F61B", "\U0001F61C", "\U0001F61D", "❌"}
	// EmojiSmile follows a country that was found.
	EmojiSmile = []string{"\U0001F600", "\U0001F60E", "\U0001F917", "\U0001F60F", "\U0001F609"}
	// EmojiSad follows a country that was not found.
	EmojiSad = []string{"\U0001F622", "\U0001F62D", "\U0001F631", "\U0001F635", "\U0001F616"}
)

// EmojiThinking follows the echo of the user's input.
const EmojiThinking = "\U0001F914"
