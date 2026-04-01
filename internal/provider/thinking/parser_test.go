package thinking

import "testing"

func TestLeadingParser_ExtractsLeadingThinkingBlock(t *testing.T) {
	parser := NewLeadingParser([]string{"<thinking>", "<think>"}, 128)

	chunks := []string{" \n<thi", "nking>plan", " carefully</thi", "nking>Hello"}
	var thinkingText string
	var regularText string
	for _, chunk := range chunks {
		result := parser.Feed(chunk)
		thinkingText += result.Thinking
		regularText += result.Text
	}
	final := parser.Finalize()
	thinkingText += final.Thinking
	regularText += final.Text

	if thinkingText != "plan carefully" {
		t.Fatalf("thinking = %q", thinkingText)
	}
	if regularText != "Hello" {
		t.Fatalf("text = %q", regularText)
	}
	if !parser.Parsed() {
		t.Fatal("expected parser to report parsed thinking")
	}
}

func TestLeadingParser_PassesThroughLiteralXMLLikeContent(t *testing.T) {
	parser := NewLeadingParser([]string{"<thinking>", "<think>"}, 128)

	input := "Literal XML example: <thinking>leave this alone</thinking>"
	result := parser.Feed(input)
	final := parser.Finalize()

	if got := result.Text + final.Text; got != input {
		t.Fatalf("text = %q", got)
	}
	if result.Thinking != "" || final.Thinking != "" {
		t.Fatalf("unexpected thinking output: %#v %#v", result, final)
	}
	if parser.Parsed() {
		t.Fatal("expected parser to keep literal XML-like text as plain text")
	}
}

func TestLeadingParser_PassesThroughIncompleteTags(t *testing.T) {
	t.Run("unterminated block", func(t *testing.T) {
		parser := NewLeadingParser([]string{"<thinking>", "<think>"}, 128)

		result := parser.Feed("<thinking>still thinking")
		final := parser.Finalize()

		if got := result.Text + final.Text; got != "<thinking>still thinking" {
			t.Fatalf("text = %q", got)
		}
		if result.Thinking != "" || final.Thinking != "" {
			t.Fatalf("unexpected thinking output: %#v %#v", result, final)
		}
		if parser.Parsed() {
			t.Fatal("expected incomplete block to remain plain text")
		}
	})

	t.Run("unfinished opening tag", func(t *testing.T) {
		parser := NewLeadingParser([]string{"<thinking>", "<think>"}, 128)

		result := parser.Feed("<thi")
		final := parser.Finalize()

		if got := result.Text + final.Text; got != "<thi" {
			t.Fatalf("text = %q", got)
		}
		if result.Thinking != "" || final.Thinking != "" {
			t.Fatalf("unexpected thinking output: %#v %#v", result, final)
		}
	})
}

func TestLeadingParser_TransitionsCleanlyIntoText(t *testing.T) {
	parser := NewLeadingParser([]string{"<thinking>", "<think>"}, 128)

	first := parser.Feed("<think>plan</think>hel")
	second := parser.Feed("lo <thinking>literal</thinking>")
	final := parser.Finalize()

	if got := first.Thinking + second.Thinking + final.Thinking; got != "plan" {
		t.Fatalf("thinking = %q", got)
	}
	if got := first.Text + second.Text + final.Text; got != "hello <thinking>literal</thinking>" {
		t.Fatalf("text = %q", got)
	}
	if !parser.Parsed() {
		t.Fatal("expected parser to stay in text mode after closing tag")
	}
}
