package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/sevigo/goframe/voice"
	"github.com/sevigo/goframe/voice/openai"
)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          Kokoro TTS - Dialogue Synthesis Demo                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Make sure Kokoro is running:")
	fmt.Println("  docker run -p 8880:8880 ghcr.io/remsky/kokoro-fastapi-cpu:latest")
	fmt.Println()

	if err := run(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	synthesizer, err := openai.NewSynthesizer(
		openai.WithBaseURL("http://localhost:8880/v1"),
		openai.WithModel("kokoro"),
		openai.WithFormat("wav"),
	)
	if err != nil {
		return fmt.Errorf("failed to create synthesizer: %w", err)
	}

	dialogueSyn := voice.NewDialogueSynthesizer(synthesizer, map[string]string{
		"Maya":  "af_bella",
		"Kenji": "am_adam",
		"Alex":  "af_sky",
	})

	dialogue := []voice.DialogueSegment{
		{Speaker: "Alex", Text: "Hey hey hey, welcome back to Cities That Never Sleep. I'm Alex, and oh boy, do we have a good one for you today."},
		{Speaker: "Maya", Text: "And I'm Maya. So Alex, I have been waiting for this episode for weeks because we are finally talking about Tokyo."},
		{Speaker: "Alex", Text: "I know! And I gotta say, Maya, ever since you came back from that trip last year, you have not shut up about it. Like, not even a little."},
		{Speaker: "Maya", Text: "Okay, that is fair, that is completely fair. But honestly? Tokyo just does something to you. Like, one moment you're standing in Akihabara with all these flashing lights and anime billboards and this wild sensory overload, and then you turn a corner and suddenly you're in front of this tiny wooden shrine with incense burning, and it's completely silent. It's like the city has multiple personalities, and somehow they all just work together."},
		{Speaker: "Alex", Text: "See, that's what gets me. It's like the city didn't choose between the future and the past. It just said, you know what, we're doing both, and we're gonna be amazing at both."},
		{Speaker: "Kenji", Text: "And honestly, that's not even an exaggeration."},
		{Speaker: "Alex", Text: "So for everyone listening, we have a very special guest today. Kenji is actually from Tokyo, born and raised, and he's been kind enough to join us and give us the insider perspective. Kenji, welcome to the show!"},
		{Speaker: "Kenji", Text: "Thanks Alex, thanks Maya. Happy to be here. And yeah, I grew up in Setagaya, which is a bit of a quieter residential area, but I spent most of my teenage years exploring every corner of that city. And I'll tell you, even after all that, Tokyo still catches me off guard sometimes."},
		{Speaker: "Maya", Text: "Okay, Kenji, I have to jump in right away because there's something I need to talk about. Shibuya Crossing. The first time I saw it in person, I literally just froze. I was standing on that corner by the Starbucks, you know the one everyone talks about, and when the light changed and all those people started moving at once, I just had my mouth open like an absolute tourist."},
		{Speaker: "Kenji", Text: "You know what's funny is I've seen that exact reaction so many times. But here's the thing, Maya, it's not just a tourist thing. Even people who live there, sometimes we'll be walking through and you just stop for a second and watch it, because it really is mesmerizing. It's like, I don't know, three thousand people all crossing at once from every direction, and somehow nobody collides? Nobody even brushes shoulders? There's this unspoken understanding of how to move."},
		{Speaker: "Alex", Text: "Wait, nobody? Like, ever?"},
		{Speaker: "Kenji", Text: "I mean, almost never. Tokyo has this rhythm, right? This flow that everyone just kind of understands. You grow up with it. It's in the way people line up for trains, the way they walk on the left side of the sidewalk, even the way they wait for the light even when there's no car coming. There's this mutual respect for shared space that's really hard to explain until you see it."},
		{Speaker: "Maya", Text: "And that actually connects to something that really surprised me. I went in expecting Tokyo to just be loud and chaotic everywhere. Like, wall to wall sensory overload all the time. But Kenji, there were moments where I felt like I was in a small village."},
		{Speaker: "Kenji", Text: "Yes! That's what people miss. So, okay, take Shimokitazawa, for example. It's like twenty minutes from Shibuya, but it feels like a completely different world. Tiny vintage shops, little cafes with maybe four seats, record stores where the owner has been curating jazz vinyl since the eighties. Or Yanaka, near Ueno, which still has this old Edo-period atmosphere. You'll see cats sleeping on fences and grandmothers sweeping their doorsteps. It's incredibly peaceful."},
		{Speaker: "Alex", Text: "Alright, alright, I need to ask the important question here. Kenji, and I want you to think carefully before you answer, where is the best ramen in Tokyo?"},
		{Speaker: "Kenji", Text: "Oh no. Oh no no no. Alex, do you have any idea what you just asked me? That question has ended friendships in Japan. Like actual friendships."},
		{Speaker: "Maya", Text: "Ha! I told you, Alex. Food in Tokyo is basically religion."},
		{Speaker: "Kenji", Text: "It really is though. And the thing is, there's no single answer because ramen in Tokyo is so regional and personal. Like, do you want tonkotsu? Shoyu? Miso? Tsukemen? And every single tiny shop, and I mean places with like eight seats and a counter, they might have been perfecting that one specific bowl for thirty, forty years. There's a place in Ebisu that only makes one type of ramen, and there's a line around the block every single day. The owner has been making the same recipe since nineteen eighty-nine."},
		{Speaker: "Alex", Text: "Okay, that's actually incredible. Thirty-seven years of the same bowl of ramen."},
		{Speaker: "Maya", Text: "And it's not just ramen though. Like, I went to this tiny sushi place in Tsukiji, after the outer market opened in the morning, and the chef, this older gentleman, he put so much care into every single piece. And the rice was warm, and the fish just melted. I'm not being dramatic, Alex, I genuinely almost cried."},
		{Speaker: "Alex", Text: "You almost cried over sushi."},
		{Speaker: "Maya", Text: "It was that good! Kenji, back me up here."},
		{Speaker: "Kenji", Text: "No, she's right. Japanese food culture is about devotion to craft. It's called shokunin kishitsu, this artisan spirit. The idea that you dedicate your life to mastering one thing. That sushi chef probably spent ten years just learning how to cook the rice before he was even allowed to touch fish."},
		{Speaker: "Alex", Text: "Ten years of rice?"},
		{Speaker: "Kenji", Text: "Ten years of rice."},
		{Speaker: "Alex", Text: "Okay, so Maya, after everything, all the scramble crossings and the hidden temples and the life-changing sushi, what's the one moment from Tokyo that really stuck with you?"},
		{Speaker: "Maya", Text: "You know what? It wasn't the big flashy things. It was this one morning, really early, like five thirty AM. I couldn't sleep because of the jet lag, so I just went for a walk. And Tokyo at dawn is a completely different city. The streets are mostly empty. You can hear birds. Shop owners are quietly setting up their displays. And I walked past this ramen shop, and the broth had been simmering all night, and this incredible smell just filled the whole block. And there was one old man sitting at the counter, eating his breakfast, totally at peace. And I thought, this is the real Tokyo. Not the neon, not Shibuya. This."},
		{Speaker: "Kenji", Text: "And that's the thing that makes me homesick. That quiet morning version of the city. That's the Tokyo I grew up in."},
		{Speaker: "Alex", Text: "Alright, I'm gonna be honest. I think about forty percent of our listeners just opened a new tab to look at flights."},
		{Speaker: "Maya", Text: "Good! But if you do go, if you actually book that trip, can I give you one piece of advice?"},
		{Speaker: "Alex", Text: "Go for it."},
		{Speaker: "Maya", Text: "Throw away your itinerary. I mean it. Don't just hit the top ten tourist spots. Get lost. Walk into a random side street. Follow a sound. Find a tiny coffee shop where the barista roasts the beans in front of you. Don't just visit Tokyo. Wander it. That's where the real magic is."},
		{Speaker: "Kenji", Text: "Perfectly said. Tokyo rewards the curious."},
		{Speaker: "Alex", Text: "What an amazing conversation. Kenji, thank you so much for joining us today and sharing your city with our listeners."},
		{Speaker: "Kenji", Text: "My pleasure. And if anyone does make it to Tokyo, let me know, I'll send you my ramen list."},
		{Speaker: "Maya", Text: "Wait, you have a list? You said there was no answer!"},
		{Speaker: "Kenji", Text: "I said it's ended friendships. I didn't say I don't have opinions."},
		{Speaker: "Alex", Text: "Ha! On that note, that's our show for today. Thanks for listening to Cities That Never Sleep. We'll catch you next time."},
	}

	ctx := context.Background()
	start := time.Now()

	fmt.Println("Synthesizing dialogue with 3 voices:")
	fmt.Println("  - Alex:  af_sky (American Female)")
	fmt.Println("  - Maya:  af_bella (American Female)")
	fmt.Println("  - Kenji: am_adam (American Male)")
	fmt.Println()

	stream, err := dialogueSyn.StreamDialogue(ctx, dialogue)
	if err != nil {
		return fmt.Errorf("failed to stream dialogue: %w", err)
	}
	defer stream.Close()

	file, err := os.Create("dialogue.wav")
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, stream)
	if err != nil {
		return fmt.Errorf("failed to write dialogue: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("✓ Dialogue synthesized in %v\n", elapsed)
	fmt.Printf("✓ Total bytes: %d\n", written)
	fmt.Printf("✓ Saved to: dialogue.wav\n")
	fmt.Println()
	fmt.Println("Play with: ffplay dialogue.wav")

	return nil
}
