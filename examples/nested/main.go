package main

import "fmt"

func main() {
	db := NewConfigOptionsDatabaseBuilder().Driver("postgres").Port(5432).TLS(true).Build()
	options := NewConfigOptionsBuilder().Host("localhost").Timeout(30).Database(db).BuildPtr()
	v := NewConfigBuilder().ID(1).Options(options).Build()
	fmt.Println(v.ID(), v.Options.Host, v.Options.Database.Driver, v.Options.Database.TLS)
}
