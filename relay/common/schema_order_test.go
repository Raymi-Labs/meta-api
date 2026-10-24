package common

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/tidwall/gjson"
)

const orderedSchema = `{"type":"object","properties":{"reasoning":{"type":"string"},"answer":{"type":"integer","maximum":9007199254740993}}}`

func TestGeminiFieldFilterPreservesSchemaAndToolResults(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	old := settings.RemoveFunctionResponseIdEnabled
	settings.RemoveFunctionResponseIdEnabled = true
	t.Cleanup(func() { settings.RemoveFunctionResponseIdEnabled = old })
	input := `{"tools":[` + orderedSchema + `],"contents":[{"parts":[{"functionResponse":{"id":"first","response":` + orderedSchema + `}},{"function_response":{"id":"second","response":9007199254740993}}]}]}`
	out, err := RemoveGeminiDisabledFields([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"tools.0", "contents.0.parts.0.functionResponse.response"} {
		if gjson.GetBytes(out, path).Raw != orderedSchema {
			t.Fatalf("schema changed at %s: %s", path, out)
		}
	}
	for _, path := range []string{"contents.0.parts.0.functionResponse.id", "contents.0.parts.1.function_response.id"} {
		if gjson.GetBytes(out, path).Exists() {
			t.Errorf("ID retained at %s", path)
		}
	}
	if gjson.GetBytes(out, "contents.0.parts.1.function_response.response").Raw != "9007199254740993" {
		t.Fatal("tool result number rounded")
	}
}

func TestDisabledFieldsPreserveSchemaOrderAndNumbers(t *testing.T) {
	input := `{"service_tier":"priority","speed":"fast","tools":[{"function":{"parameters":` + orderedSchema + `}}],"stream_options":{"include_obfuscation":false,"nested":` + orderedSchema + `}}`
	out, err := RemoveDisabledFields([]byte(input), dto.ChannelOtherSettings{}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"tools.0.function.parameters", "stream_options.nested"} {
		if got := gjson.GetBytes(out, path).Raw; got != orderedSchema {
			t.Errorf("%s changed: %s", path, got)
		}
	}
	if gjson.GetBytes(out, "service_tier").Exists() || gjson.GetBytes(out, "speed").Exists() || gjson.GetBytes(out, "stream_options.include_obfuscation").Exists() {
		t.Fatalf("disabled fields remain: %s", out)
	}
}

func TestParamOperationsPreserveOriginalSchema(t *testing.T) {
	for _, operation := range []string{"copy", "move", "append", "prepend"} {
		t.Run(operation, func(t *testing.T) {
			input := `{"source":` + orderedSchema + `,"tools":[` + orderedSchema + `]}`
			var got string
			var err error
			switch operation {
			case "copy":
				got, err = copyValue(input, "source", "target")
			case "move":
				got, err = moveValue(input, "source", "target")
			default:
				got, err = modifyArray(input, "tools", map[string]any{"type": "new"}, operation == "prepend")
			}
			if err != nil {
				t.Fatal(err)
			}
			path := "target"
			if operation == "append" {
				path = "tools.0"
			} else if operation == "prepend" {
				path = "tools.1"
			}
			if gjson.Get(got, path).Raw != orderedSchema {
				t.Fatalf("schema reordered or number rounded: %s", got)
			}
			if operation == "move" && gjson.Get(got, "source").Exists() {
				t.Fatal("move did not remove source")
			}
		})
	}
	for _, value := range []string{`"a\\nb"`, `9007199254740993`, `null`, `false`} {
		got, err := copyValue(`{"source":`+value+`}`, "source", "target")
		if err != nil || !strings.Contains(got, `"target":`+value) {
			t.Fatalf("scalar copy changed %s: %s, %v", value, got, err)
		}
	}
}
