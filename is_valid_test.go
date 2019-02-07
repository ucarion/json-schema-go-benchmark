package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	impl1 "github.com/json-schema-spec/json-schema-go"
	impl4 "github.com/qri-io/jsonschema"
	impl3 "github.com/santhosh-tekuri/jsonschema"
	impl2 "github.com/xeipuuv/gojsonschema"

	"github.com/tidwall/sjson"
)

var realisticSchema1Bytes = []byte(`
{
	"type": "object",
	"required": ["event", "userId", "properties"],
	"properties": {
		"event": {
			"type": "string"
		},
		"userId": {
			"type": "string"
		},
		"properties": {
			"type": "object",
			"required": ["products", "coupon", "total"],
			"properties": {
				"products": {
					"type": "array",
					"items": {
						"type": "object",
						"required": ["id", "variant", "quantity", "price"],
						"properties": {
							"id": {
								"type": "string"
							},
							"variant": {
								"type": "string"
							},
							"quantity": {
								"type": "integer"
							},
							"price": {
								"type": "number"
							}
						}
					}
				},
				"coupon": {
					"type": "string"
				},
				"total": {
					"type": "number"
				}
			}
		}
	}
}
`)

type benchCase struct {
	name        string
	schemaBytes []byte
	schemaJSON  map[string]interface{}
	instances   []benchCaseInstance
}

type benchCaseInstance struct {
	bytes []byte
	json  map[string]interface{}
	valid bool
}

func genInstances1(seed int64, n int) []benchCaseInstance {
	events := make([]string, n)
	valids := make([]bool, n)
	rng := rand.New(rand.NewSource(seed))

	for i := 0; i < n; i++ {
		events[i] = `{}`

		events[i], _ = sjson.Set(events[i], "event", "Order Completed")
		if rng.Float32() < 0.5 {
			continue
		}

		events[i], _ = sjson.Set(events[i], "userId", "foobar")
		if rng.Float32() < 0.5 {
			continue
		}

		loops := rng.Int() % 1000
		loopOk := loops > 0
		for j := 0; j < loops; j++ {
			events[i], _ = sjson.Set(events[i], fmt.Sprintf("properties.products.%d.id", j), "xxx")
			events[i], _ = sjson.Set(events[i], fmt.Sprintf("properties.products.%d.variant", j), "xxx")
			if rng.Float32() < 0.5 {
				loopOk = false
				continue
			}

			events[i], _ = sjson.Set(events[i], fmt.Sprintf("properties.products.%d.quantity", j), 5)

			if rng.Float32() < 0.5 {
				// err: price as string instead of number
				events[i], _ = sjson.Set(events[i], fmt.Sprintf("properties.products.%d.price", j), "xxx")
				loopOk = false
			} else {
				events[i], _ = sjson.Set(events[i], fmt.Sprintf("properties.products.%d.price", j), 3.14)
			}
		}

		if rng.Float32() < 0.5 {
			continue
		}

		events[i], _ = sjson.Set(events[i], "properties.coupon", "asdf")
		events[i], _ = sjson.Set(events[i], "properties.total", 42)
		valids[i] = loopOk
	}

	instances := make([]benchCaseInstance, n)
	for i, event := range events {
		var eventJSON map[string]interface{}
		err := json.Unmarshal([]byte(event), &eventJSON)
		if err != nil {
			panic(err)
		}

		instances[i] = benchCaseInstance{
			bytes: []byte(event),
			json:  eventJSON,
			valid: valids[i],
		}
	}

	return instances
}

const seed1 = int64(11664987322298)

func BenchmarkIsValid(b *testing.B) {
	var realisticSchema1JSON map[string]interface{}
	err := json.Unmarshal(realisticSchema1Bytes, &realisticSchema1JSON)
	if err != nil {
		panic(err)
	}

	realistic1 := genInstances1(seed1, 1)
	realistic10 := genInstances1(seed1, 10)
	realistic100 := genInstances1(seed1, 100)
	realistic1000 := genInstances1(seed1, 1000)

	benchCases := []benchCase{
		benchCase{
			"realistic 1",
			realisticSchema1Bytes,
			realisticSchema1JSON,
			realistic1,
		},
		benchCase{
			"realistic 10",
			realisticSchema1Bytes,
			realisticSchema1JSON,
			realistic10,
		},
		benchCase{
			"realistic 100",
			realisticSchema1Bytes,
			realisticSchema1JSON,
			realistic100,
		},
		benchCase{
			"realistic 1000",
			realisticSchema1Bytes,
			realisticSchema1JSON,
			realistic1000,
		},
	}

	for _, bb := range benchCases {
		b.Run(fmt.Sprintf("impl1-one/%s", bb.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				schemas := []map[string]interface{}{bb.schemaJSON}
				config := impl1.ValidatorConfig{MaxErrors: 1}
				validator, err := impl1.NewValidatorWithConfig(schemas, config)

				if err != nil {
					b.Fatalf("err: %s", err.Error())
				}

				for _, instance := range bb.instances {
					result, err := validator.Validate(instance.json)
					if err != nil {
						b.Fatal(err)
					}

					if result.IsValid() != instance.valid {
						b.Fatalf("incorrect result: got %t expected %t", result.IsValid(), instance.valid)
					}
				}
			}
		})

		b.Run(fmt.Sprintf("impl1-all/%s", bb.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				schemas := []map[string]interface{}{bb.schemaJSON}
				config := impl1.ValidatorConfig{MaxErrors: 0}
				validator, err := impl1.NewValidatorWithConfig(schemas, config)

				if err != nil {
					b.Fatalf("err: %s", err.Error())
				}

				for _, instance := range bb.instances {
					result, err := validator.Validate(instance.json)
					if err != nil {
						b.Fatal(err)
					}

					if result.IsValid() != instance.valid {
						b.Fatalf("incorrect result: got %t expected %t", result.IsValid(), instance.valid)
					}
				}
			}
		})

		b.Run(fmt.Sprintf("impl2/%s", bb.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				schemaLoader := impl2.NewGoLoader(bb.schemaJSON)

				for _, instance := range bb.instances {
					documentLoader := impl2.NewGoLoader(instance.json)
					result, err := impl2.Validate(schemaLoader, documentLoader)
					if err != nil {
						b.Fatal(err)
					}

					if result.Valid() != instance.valid {
						b.Fatalf("incorrect result: got %t expected %t", result.Valid(), instance.valid)
					}
				}
			}
		})

		b.Run(fmt.Sprintf("impl3/%s", bb.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				compiler := impl3.NewCompiler()
				err = compiler.AddResource("foo.json", bytes.NewReader(bb.schemaBytes))
				if err != nil {
					b.Fatal(err)
				}

				validator, err := compiler.Compile("foo.json")
				if err != nil {
					b.Fatal(err)
				}

				for _, instance := range bb.instances {
					reader := bytes.NewReader(instance.bytes)
					err := validator.Validate(reader)

					var valid bool
					if err == nil {
						valid = true
					} else {
						if _, ok := err.(*impl3.ValidationError); ok {
							valid = false
						} else {
							b.Fatal(err)
						}
					}

					if valid != instance.valid {
						b.Fatalf("incorrect result: got %t expected %t", valid, instance.valid)
					}
				}
			}
		})

		b.Run(fmt.Sprintf("impl4/%s", bb.name), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				validator4 := impl4.RootSchema{}
				err = json.Unmarshal(bb.schemaBytes, &validator4)
				if err != nil {
					b.Fatalf("err: %s", err.Error())
				}

				for _, instance := range bb.instances {
					result, err := validator4.ValidateBytes(instance.bytes)
					if err != nil {
						b.Fatal(err)
					}

					valid := len(result) == 0
					if valid != instance.valid {
						b.Fatalf("incorrect result: got %t expected %t", valid, instance.valid)
					}
				}
			}
		})
	}
}
