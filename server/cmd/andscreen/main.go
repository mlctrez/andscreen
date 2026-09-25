// Command andscreen serves the time and temperature screen.
package main

import (
	"github.com/mlctrez/andscreen/internal/app"
	"github.com/mlctrez/servicego"
)

func main() {
	servicego.Run(&app.Service{})
}
