// persona.go holds the bundled system persona for the standalone chat process.
// It stays deliberately plain: no scene content, no state numbers.
package chat

// NEUTRAL_PERSONA is the fallback system prompt when neither a persona file nor
// config.persona is set. It carries no character, scene or state vocabulary:
// those are data.
const NEUTRAL_PERSONA = "只依据提供的状态与记忆作答，不编造未提供的信息；" +
	"缺少某个信息时直接说明不知道。"
