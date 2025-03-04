package main

import (
	"fmt"
	"html"
	"strings"
)

type Converter struct {
	input  []rune
	pos    int
	output *strings.Builder
	stack  []string
}

func NewConverter(input string) *Converter {
	var sb strings.Builder
	sb.Grow(len(input) * 2)

	return &Converter{
		input:  []rune(input),
		output: &sb,
		stack:  make([]string, 0),
	}
}

func (c *Converter) push(tag string) {
	c.stack = append(c.stack, tag)
	c.output.WriteString("<" + tag + ">")
}

func (c *Converter) pop() (string, bool) {
	if len(c.stack) == 0 {
		return "", false
	}
	tag := c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	c.output.WriteString("</" + tag + ">")
	return tag, true
}

func (c *Converter) peek() string {
	if len(c.stack) == 0 {
		return ""
	}
	return c.stack[len(c.stack)-1]
}

func (c *Converter) write(r rune) {
	switch r {
	case '<':
		c.output.WriteString("&lt;")
	case '>':
		c.output.WriteString("&gt;")
	case '&':
		c.output.WriteString("&amp;")
	default:
		c.output.WriteRune(r)
	}
}

func (c *Converter) Convert() string {
	for c.pos < len(c.input) {
		curr := c.input[c.pos]

		switch curr {
		case '\\':
			if c.pos+1 < len(c.input) {
				c.write(c.input[c.pos+1])
				c.pos += 2
			} else {
				c.write(curr)
				c.pos++
			}

		case '*':
			if c.checkAhead("**") {
				if c.peek() == "b" {
					c.pop()
				} else {
					c.push("b")
				}
				c.pos += 2
			} else {
				if c.peek() == "i" {
					c.pop()
				} else {
					c.push("i")
				}
				c.pos++
			}

		case '`':
			if c.checkAhead("```") {
				if c.peek() == "pre" {
					c.pop()
				} else {
					c.push("pre")
					c.skipLang()
				}
				c.pos += 3
			} else {
				if c.peek() == "code" {
					c.pop()
				} else {
					c.push("code")
				}
				c.pos++
			}

		case '[':
			if link := c.processLink(); link != "" {
				c.output.WriteString(link)
			} else {
				c.write(curr)
				c.pos++
			}

		default:
			c.write(curr)
			c.pos++
		}
	}

	for len(c.stack) > 0 {
		c.pop()
	}

	return c.output.String()
}

// TODO
func (c *Converter) skipLang() {
}

func (c *Converter) checkAhead(pattern string) bool {
	patternRunes := []rune(pattern)
	if c.pos+len(patternRunes) > len(c.input) {
		return false
	}
	for i, r := range patternRunes {
		if c.input[c.pos+i] != r {
			return false
		}
	}

	return true
}

func (c *Converter) processLink() string {
	textEnd := -1
	for i := c.pos + 1; i < len(c.input); i++ {
		if c.input[i] == ']' {
			textEnd = i
			break
		}
	}

	if textEnd == -1 || textEnd+2 >= len(c.input) {
		c.pos++
		return ""
	}

	if c.input[textEnd+1] != '(' {
		c.pos++
		return ""
	}

	text := string(c.input[c.pos+1 : textEnd])
	urlStart := textEnd + 2
	urlEnd := -1
	for i := urlStart; i < len(c.input); i++ {
		if c.input[i] == ')' {
			urlEnd = i
			break
		}
	}

	if urlEnd == -1 {
		c.pos++
		return ""
	}

	url := string(c.input[urlStart:urlEnd])
	if !strings.ContainsAny(url, ":./) ") {
		c.pos++
		return ""
	}

	c.pos = urlEnd + 1
	return fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(url), html.EscapeString(text))
}
