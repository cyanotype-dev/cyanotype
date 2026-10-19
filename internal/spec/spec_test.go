package spec

import (
	"reflect"
	"testing"
)

func TestWithDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Spec
		want Spec
	}{
		{
			name: "empty spec gets every default",
			in:   Spec{},
			want: Spec{
				Mode: ModeAdvanced,
				Languages: Languages{
					Code: "english", Comments: "english", Docs: "english",
					Commits: "english", AgentReplies: "english",
				},
				Output: Output{Split: SplitAuto, MaxLines: 100, TopicDir: "docs/agents"},
			},
		},
		{
			name: "values already set are kept",
			in: Spec{
				Mode:      ModeSimple,
				Languages: Languages{AgentReplies: "italian"},
				I18n:      &I18n{DefaultLocale: "en", Locales: []string{"en", "it"}, TranslationsBy: TranslationsHuman},
				Output:    Output{Split: SplitNever, MaxLines: 60, TopicDir: "agents"},
			},
			want: Spec{
				Mode: ModeSimple,
				Languages: Languages{
					Code: "english", Comments: "english", Docs: "english",
					Commits: "english", AgentReplies: "italian",
				},
				I18n:   &I18n{DefaultLocale: "en", Locales: []string{"en", "it"}, TranslationsBy: TranslationsHuman},
				Output: Output{Split: SplitNever, MaxLines: 60, TopicDir: "agents"},
			},
		},
		{
			name: "i18n defaults to the default locale only, drafted by the agent",
			in:   Spec{I18n: &I18n{DefaultLocale: "it"}},
			want: Spec{
				Mode: ModeAdvanced,
				Languages: Languages{
					Code: "english", Comments: "english", Docs: "english",
					Commits: "english", AgentReplies: "english",
				},
				I18n:   &I18n{DefaultLocale: "it", Locales: []string{"it"}, TranslationsBy: TranslationsAgentDraft},
				Output: Output{Split: SplitAuto, MaxLines: 100, TopicDir: "docs/agents"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.WithDefaults(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WithDefaults() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

// TestWithDefaultsLeavesInputAlone guards against filling defaults into the
// spec that Save writes back.
func TestWithDefaultsLeavesInputAlone(t *testing.T) {
	in := Spec{I18n: &I18n{DefaultLocale: "en"}}
	in.WithDefaults()
	if want := (I18n{DefaultLocale: "en"}); !reflect.DeepEqual(*in.I18n, want) {
		t.Errorf("input i18n = %+v after WithDefaults, want %+v", *in.I18n, want)
	}
}
