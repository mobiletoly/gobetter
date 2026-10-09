package main

//gobetter:struct builder
type Config struct {
	//gobetter:field name=ID,getter
	_id     int64
	Options *struct {
		Host     string `json:"host"`
		Timeout  int    `json:"timeout"`
		Database struct {
			Driver string `json:"driver"`
			Port   int    `json:"port"`
			//gobetter:field optional
			TLS bool `json:"tls"`
		} `json:"database"`
	} `json:"options"`
}
