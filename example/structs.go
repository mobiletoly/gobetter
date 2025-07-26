package example

import (
	"time"
)

//go:generate gobetter -input $GOFILE

// Person represents a person with basic information
type Person struct { //+gob:Constructor
	firstName string    //+gob:getter
	lastName  string    //+gob:getter
	dob       time.Time //+gob:getter +gob:acronym
	Email     string
	Phone     *string //+gob:_
	Bio       *string //+gob:_
}

// Company represents a business entity
type Company struct { //+gob:Constructor
	Name        string  `json:"name"`
	Industry    string  `json:"industry"`
	Founded     int     `json:"founded"`
	Employees   int     `json:"employees"`
	Revenue     float64 `json:"revenue"`
	IsPublic    bool    `json:"is_public"`
	Website     *string `json:"website"`     //+gob:_
	Description *string `json:"description"` //+gob:_
}

// Employee represents an employee with company relationship
type Employee struct { //+gob:Constructor
	person    Person     //+gob:getter
	company   Company    //+gob:getter
	position  string     //+gob:getter
	Salary    float64    `json:"salary"`
	StartDate time.Time  `json:"start_date"`
	EndDate   *time.Time `json:"end_date"` //+gob:_
}

// Product represents a product in an e-commerce system
type Product struct { //+gob:Constructor
	name        string //+gob:getter
	sku         string //+gob:getter
	Price       float64
	Category    string
	Tags        []string
	Attributes  map[string]string
	InStock     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Description *string //+gob:_
}

// Order represents a customer order
type Order struct { //+gob:Constructor
	orderID     string //+gob:getter
	customerID  int64  //+gob:getter
	Products    []Product
	TotalAmount float64
	Status      string
	OrderDate   time.Time
	ShippedDate *time.Time //+gob:_
}

// UserProfile combines multiple entities
type UserProfile struct { //+gob:Constructor
	user        Person //+gob:getter
	Orders      []Order
	Preferences map[string]interface{}
	CreatedAt   time.Time
	LastLogin   *time.Time //+gob:_
	IsVerified  bool
}

// NestedStructExample demonstrates nested inner structs
type NestedStructExample struct { //+gob:Constructor
	id   int64  //+gob:getter
	name string //+gob:getter

	Config *struct { //+gob:Constructor
		Host    string `json:"host"`
		Port    int    `json:"port"`
		Timeout int    `json:"timeout"`

		// Database settings nested within Config
		Database struct { //+gob:Constructor
			Driver  string `json:"driver"`
			Host    string `json:"host"`
			Port    int
			Name    string
			SslMode bool //+gob:_
		} `json:"database"`
	} `json:"config"`

	IsActive bool
}
