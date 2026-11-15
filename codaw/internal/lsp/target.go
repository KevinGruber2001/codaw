package lsp

import (
	"fmt"
	"regexp"
	"strconv"
)

var fxTargetPattern = regexp.MustCompile(`^fx\[(\d+)\]\.(.+)$`)

// target is what an automation lane's target = "…" points at in its track file.
type target struct {
	text   string
	index  int    // of the effect; -1 for the track's own gain and pan
	param  string // "gain", "pan" or the effect's parameter
	fx     *table // nil when there is no such effect
	fxType string
	key    key // the parameter's documentation, if it is a known one
	known  bool
}

// resolveTarget reads a target; the error says what is wrong with it.
func resolveTarget(d *document, text string) (target, error) {
	t := target{text: text, index: -1, param: text}
	if text == "gain" || text == "pan" {
		t.key, t.known = findKey(sections[kindTrack][""], text)
		return t, nil
	}
	m := fxTargetPattern.FindStringSubmatch(text)
	if m == nil {
		return t, fmt.Errorf(`unknown target %q: use "gain", "pan" or "fx[N].param"`, text)
	}
	t.index, _ = strconv.Atoi(m[1])
	t.param = m[2]
	fx := d.all("fx")
	if t.index >= len(fx) {
		return t, fmt.Errorf("fx[%d] does not exist: this track has %s", t.index, count(len(fx), "effect"))
	}
	t.fx = fx[t.index]
	t.fxType = t.fx.text("type")
	if effect, ok := effects[t.fxType]; ok {
		if t.key, t.known = findKey(effect.params, t.param); !t.known {
			return t, fmt.Errorf("%s has no parameter %q", t.fxType, t.param)
		}
	}
	return t, nil
}

// describe says what the target is, in a sentence fragment: "`low_db` of the 2nd effect, `eq_3band`".
func (t target) describe() string {
	if t.index < 0 {
		return fmt.Sprintf("the track's `%s`", t.param)
	}
	fxType := t.fxType
	if fxType == "" {
		fxType = "?"
	}
	return fmt.Sprintf("`%s` of the %s effect, `%s`", t.param, ordinal(t.index+1), fxType)
}

// now is the value the target has in the file, or its default.
func (t target) now(d *document) string {
	from := d.tables[0]
	if t.index >= 0 {
		from = t.fx
	}
	if from != nil {
		if e := from.entry(t.param); e != nil && e.value != "" {
			return "now `" + e.value + "`"
		}
	}
	return "not set"
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}

func count(n int, noun string) string {
	switch n {
	case 0:
		return "no " + noun + "s"
	case 1:
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
