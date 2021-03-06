package kibana

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/beats/libbeat/common"
)

func TestNewGenerator(t *testing.T) {
	beatDir := tmpPath()
	defer teardown(beatDir)

	// checks for fields.yml
	generator, err := NewGenerator("beat-index", "mybeat.", filepath.Join(beatDir, "notexistent"), "7.0")
	assert.Error(t, err)

	generator, err = NewGenerator("beat-index", "mybeat.", beatDir, "7.0")
	assert.NoError(t, err)
	assert.Equal(t, "7.0", generator.version)
	assert.Equal(t, "beat-index", generator.indexName)
	assert.Equal(t, filepath.Join(beatDir, "fields.yml"), generator.fieldsYaml)

	// creates file dir and sets name
	expectedDir := filepath.Join(beatDir, "_meta/kibana/default/index-pattern")
	assert.Equal(t, expectedDir, generator.targetDirDefault)
	_, err = os.Stat(generator.targetDirDefault)
	assert.NoError(t, err)

	expectedDir = filepath.Join(beatDir, "_meta/kibana/5.x/index-pattern")
	assert.Equal(t, expectedDir, generator.targetDir5x)
	_, err = os.Stat(generator.targetDir5x)
	assert.NoError(t, err)

	assert.Equal(t, "mybeat.json", generator.targetFilename)
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: " beat index pattern", expected: "beatindexpattern"},
		{input: "Beat@Index.!", expected: "BeatIndex"},
		{input: "beatIndex", expected: "beatIndex"},
	}
	for idx, test := range tests {
		output := clean(test.input)
		msg := fmt.Sprintf("(%v): Expected <%s> Received: <%s>", idx, test.expected, output)
		assert.Equal(t, test.expected, output, msg)
	}
}

func TestGenerateFieldsYaml(t *testing.T) {
	beatDir := tmpPath()
	defer teardown(beatDir)
	generator, err := NewGenerator("metricbeat-*", "metric beat ?!", beatDir, "7.0.0-alpha1")
	_, err = generator.Generate()
	assert.NoError(t, err)

	generator.fieldsYaml = ""
	_, err = generator.Generate()
	assert.Error(t, err)
}

func TestDumpToFile5x(t *testing.T) {
	beatDir := tmpPath()
	defer teardown(beatDir)
	generator, err := NewGenerator("metricbeat-*", "metric beat ?!", beatDir, "7.0.0-alpha1")
	_, err = generator.Generate()
	assert.NoError(t, err)

	generator.targetDir5x = "./non-existing/something"
	_, err = generator.Generate()
	assert.Error(t, err)
}

func TestDumpToFileDefault(t *testing.T) {
	beatDir := tmpPath()
	defer teardown(beatDir)
	generator, err := NewGenerator("metricbeat-*", "metric beat ?!", beatDir, "7.0.0-alpha1")
	_, err = generator.Generate()
	assert.NoError(t, err)

	generator.targetDirDefault = "./non-existing/something"
	_, err = generator.Generate()
	assert.Error(t, err)
}

func TestGenerate(t *testing.T) {
	beatDir := tmpPath()
	defer teardown(beatDir)
	generator, err := NewGenerator("beat-*", "b eat ?!", beatDir, "7.0.0-alpha1")
	pattern, err := generator.Generate()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(pattern))

	tests := []map[string]string{
		{"existing": "beat-5x.json", "created": "_meta/kibana/5.x/index-pattern/beat.json"},
		{"existing": "beat-default.json", "created": "_meta/kibana/default/index-pattern/beat.json"},
	}
	testGenerate(t, beatDir, tests)
}

func TestGenerateExtensive(t *testing.T) {
	beatDir, err := filepath.Abs("./testdata/extensive")
	if err != nil {
		panic(err)
	}
	defer teardown(beatDir)
	generator, err := NewGenerator("metricbeat-*", "metric be at ?!", beatDir, "7.0.0-alpha1")
	pattern, err := generator.Generate()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(pattern))

	tests := []map[string]string{
		{"existing": "metricbeat-5x.json", "created": "_meta/kibana/5.x/index-pattern/metricbeat.json"},
		{"existing": "metricbeat-default.json", "created": "_meta/kibana/default/index-pattern/metricbeat.json"},
	}
	testGenerate(t, beatDir, tests)
}

func testGenerate(t *testing.T, beatDir string, tests []map[string]string) {
	for _, test := range tests {
		// compare default
		existing, err := readJson(filepath.Join(beatDir, test["existing"]))
		assert.NoError(t, err)
		created, err := readJson(filepath.Join(beatDir, test["created"]))
		assert.NoError(t, err)

		var attrExisting, attrCreated common.MapStr

		if strings.Contains(test["existing"], "default") {
			assert.Equal(t, existing["version"], created["version"])

			objExisting := existing["objects"].([]interface{})[0].(map[string]interface{})
			objCreated := created["objects"].([]interface{})[0].(map[string]interface{})

			assert.Equal(t, objExisting["version"], objCreated["version"])
			assert.Equal(t, objExisting["id"], objCreated["id"])
			assert.Equal(t, objExisting["type"], objCreated["type"])

			attrExisting = objExisting["attributes"].(map[string]interface{})
			attrCreated = objCreated["attributes"].(map[string]interface{})
		} else {
			attrExisting = existing
			attrCreated = created
		}

		// check fieldFormatMap
		var ffmExisting, ffmCreated map[string]interface{}
		err = json.Unmarshal([]byte(attrExisting["fieldFormatMap"].(string)), &ffmExisting)
		assert.NoError(t, err)
		err = json.Unmarshal([]byte(attrCreated["fieldFormatMap"].(string)), &ffmCreated)
		assert.NoError(t, err)
		assert.Equal(t, ffmExisting, ffmCreated)

		// check fields
		var fieldsExisting, fieldsCreated []map[string]interface{}
		err = json.Unmarshal([]byte(attrExisting["fields"].(string)), &fieldsExisting)
		assert.NoError(t, err)
		err = json.Unmarshal([]byte(attrCreated["fields"].(string)), &fieldsCreated)
		assert.NoError(t, err)
		assert.Equal(t, len(fieldsExisting), len(fieldsCreated))
		for _, e := range fieldsExisting {
			idx := find(fieldsCreated, e["name"].(string))
			assert.NotEqual(t, -1, idx)
			assert.Equal(t, e, fieldsCreated[idx])
		}
	}
}

func find(a []map[string]interface{}, k string) int {
	for idx, e := range a {
		if e["name"].(string) == k {
			return idx
		}
	}
	return -1
}

func readJson(path string) (map[string]interface{}, error) {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	err = json.Unmarshal(f, &data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func tmpPath() string {
	beatDir, err := filepath.Abs("./testdata")
	if err != nil {
		panic(err)
	}
	return beatDir
}

func teardown(path string) {
	if path == "" {
		path = tmpPath()
	}
	os.RemoveAll(filepath.Join(path, "_meta"))
}
