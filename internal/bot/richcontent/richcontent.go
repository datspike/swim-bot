// Package richcontent extracts moderation signals from Telegram RichMessage trees.
package richcontent

import (
	"strings"

	tele "gopkg.in/telebot.v4"
)

// Text flattens a rich message into searchable plain text.
func Text(message *tele.RichMessage) string {
	if message == nil {
		return ""
	}
	return strings.Join(blocksText(message.Blocks), "\n")
}

// HasLink reports whether any rich text node contains a URL entity.
func HasLink(message *tele.RichMessage) bool {
	if message == nil {
		return false
	}
	return blocksHaveLink(message.Blocks)
}

func blocksText(blocks []tele.RichBlock) []string {
	parts := make([]string, 0, len(blocks))
	for i := range blocks {
		parts = append(parts, blockText(&blocks[i])...)
	}
	return parts
}

func blockText(block *tele.RichBlock) []string {
	var parts []string
	appendText := func(text *tele.RichText) {
		if text == nil {
			return
		}
		if value := strings.TrimSpace(text.String()); value != "" {
			parts = append(parts, value)
		}
	}

	appendText(block.Text)
	if value := strings.TrimSpace(block.Expression); value != "" {
		parts = append(parts, value)
	}
	appendText(block.Summary)
	appendText(block.Credit)
	appendText(block.TableCaption)
	if block.Caption != nil {
		appendText(&block.Caption.Text)
		appendText(block.Caption.Credit)
	}

	for _, row := range block.Cells {
		var cells []string
		for _, cell := range row {
			if cell.Text == nil {
				continue
			}
			if value := strings.TrimSpace(cell.Text.String()); value != "" {
				cells = append(cells, value)
			}
		}
		if len(cells) > 0 {
			parts = append(parts, strings.Join(cells, " "))
		}
	}

	parts = append(parts, blocksText(block.Blocks)...)
	for _, item := range block.Items {
		parts = append(parts, blocksText(item.Blocks)...)
	}
	return parts
}

func blocksHaveLink(blocks []tele.RichBlock) bool {
	for i := range blocks {
		if blockHasLink(&blocks[i]) {
			return true
		}
	}
	return false
}

func blockHasLink(block *tele.RichBlock) bool {
	if richTextHasLink(block.Text) ||
		richTextHasLink(block.Summary) ||
		richTextHasLink(block.Credit) ||
		richTextHasLink(block.TableCaption) {
		return true
	}
	if block.Caption != nil &&
		(richTextHasLink(&block.Caption.Text) || richTextHasLink(block.Caption.Credit)) {
		return true
	}
	for _, row := range block.Cells {
		for _, cell := range row {
			if richTextHasLink(cell.Text) {
				return true
			}
		}
	}
	if blocksHaveLink(block.Blocks) {
		return true
	}
	for _, item := range block.Items {
		if blocksHaveLink(item.Blocks) {
			return true
		}
	}
	return false
}

func richTextHasLink(text *tele.RichText) bool {
	if text == nil {
		return false
	}
	if text.Type == tele.RichURL {
		return true
	}
	if richTextHasLink(text.Text) {
		return true
	}
	for i := range text.Parts {
		if richTextHasLink(&text.Parts[i]) {
			return true
		}
	}
	return false
}
