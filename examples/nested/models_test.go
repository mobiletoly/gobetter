package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNestedAliasesPointersAndTags(t *testing.T) {
	db := NewConfigOptionsDatabaseBuilder().Driver("postgres").Port(5432).Build()
	if db.TLS {
		t.Fatal("optional field must default to zero")
	}
	options := NewConfigOptionsBuilder().Host("localhost").Timeout(30).Database(db).BuildPtr()
	v := NewConfigBuilder().ID(9).Options(options).Build()
	if v.ID() != 9 || v.Options.Database.Port != 5432 {
		t.Fatal(v)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	opts := decoded["options"].(map[string]any)
	database := opts["database"].(map[string]any)
	if opts["host"] != "localhost" || database["driver"] != "postgres" {
		t.Fatal(string(data))
	}
	if _, ok := reflect.TypeFor[Config]().FieldByName("_id"); !ok {
		t.Fatal("original field name changed")
	}
	options.Host = "shared"
	if v.Options.Host != "shared" {
		t.Fatal("pointer field should remain shared")
	}
}
