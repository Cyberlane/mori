package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Cyberlane/mori/internal/language"
	"github.com/Cyberlane/mori/internal/source"
)

func TestCConsumerGrammarForms(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"typed_widget_listener": `ZMK_DISPLAY_WIDGET_LISTENER(widget, struct status, update, get)
int value(void) { return 1; }`,
		"section_iteration": `int value(void) { STRUCT_SECTION_FOREACH(item_type, item) { if (item->active) { return 1; } } return 0; }`,
		"ordinary_preprocessor_after_if": `int value(int x) { if (x) { work(); }
#if FEATURE && OTHER
if (ready()) { work(); }
#endif
#if OTHER
int y = 1;
if (y) { work(); }
#endif
return 0; }`,
		"conditional_else": `int value(int x) { if (x) { return 1; }
#if FEATURE
else { return 2; }
#endif
return 0; }`,
		"initializer_alternatives": `int value(void) { int items[] = { 1,
#if IS_ENABLED(CONFIG_FEATURE)
2,
#else
3,
#endif
4 }; return items[0]; }`,
		"nested_initializer_alternatives": `int value(void) { int items[] = {
#ifdef FEATURE
#if LEVEL > 1
2,
#endif
#else
3,
#endif
4 }; return items[0]; }`,
		"static_declaration_macro": `static K_WORK_DELAYABLE_DEFINE(work_item, callback);
int value(void) { return 1; }`,
		"instance_iteration_macro": `DT_INST_FOREACH_STATUS_OKAY(MAKE_INSTANCE)
int value(void) { return 1; }`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.c")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.c", Language: spec}, 1)
			if len(warnings) != 0 || len(fragments) != 1 {
				t.Fatalf("fragments/warnings = %+v / %+v", fragments, warnings)
			}
		})
	}
}

func TestCConsumerGrammarKeepsMalformedSyntaxVisible(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"typed_widget_listener_missing_type": `ZMK_DISPLAY_WIDGET_LISTENER(widget, struct, update, get)
int value(void) { return 1; }`,
		"section_iteration_missing_argument": `int value(void) { STRUCT_SECTION_FOREACH(item_type,) { return 1; } return 0; }`,
		"conditional_else_invalid_body": `int value(int x) { if (x) { return 1; }
#if FEATURE
else { return +; }
#endif
return 0; }`,
		"else_after_complete_if_else": `int value(int x) { if (x) {} else {}
#if FEATURE
else { return 3; }
#endif
return 0; }`,
		"repeated_conditional_else": `int value(int x) { if (x) { return 1; }
#if FEATURE
else { return 2; } else { return 3; }
#endif
return 0; }`,
		"bad_initializer_branch": `int value(void) { int items[] = { 1,
#if FEATURE
2 + ,
#else
3,
#endif
4 }; return items[0]; }`,
		"missing_endif": `int value(void) { int items[] = { 1,
#if FEATURE
2,
4 }; return items[0]; }`,
		"missing_macro_semicolon": `static K_WORK_DELAYABLE_DEFINE(work_item, callback)
int value(void) { return 1; }`,
		"invalid_macro_argument": `static K_WORK_DELAYABLE_DEFINE(work_item,, callback);
int value(void) { return 1; }`,
		"invalid_iteration_argument": `DT_INST_FOREACH_STATUS_OKAY(,)
int value(void) { return 1; }`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.c")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			spec, _ := language.Detect(path)
			fragments, warnings := File(context.Background(), source.File{Path: path, DisplayPath: "input.c", Language: spec}, 1)
			if len(warnings) == 0 {
				t.Fatal("malformed C has no warning")
			}
			if (name == "bad_initializer_branch" || name == "missing_endif") && len(fragments) != 0 {
				t.Fatalf("invalid function scored: %+v", fragments)
			}
		})
	}
}
