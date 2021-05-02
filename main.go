package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/talos-systems/wglan-manager/db"
	"github.com/talos-systems/wglan-manager/types"
)

var listenAddr = ":3000"
var devMode bool

var nodeDB db.DB

func init() {
	flag.StringVar(&listenAddr, "addr", ":3000", "addr on which to listen")
	flag.BoolVar(&devMode, "debug", false, "enable debug mode")
}

func main() {
   
	flag.Parse()

	if os.Getenv("MODE") == "dev" {
		devMode = true
	}

   nodeDB = db.New()

   app := fiber.New()

   app.Get("/:id", func(c *fiber.Ctx) error {
      n := nodeDB.Get(c.Get("id", ""))
      if n == nil {
         return c.SendStatus(http.StatusNotFound)
      }

      return c.JSON(n)

   })

   app.Post("/:id", func(c *fiber.Ctx) error {
      n := new(types.Node)

      if err := c.BodyParser(n); err != nil {
         return c.SendStatus(http.StatusBadRequest)
      }

      if err := nodeDB.AddEndpoints(n.ID, n.KnownEndpoints); err != nil {
         log.Printf("failed to add endpoints %q to node %s: %v", strings.Join(n.KnownEndpoints,","), n.ID, err)
         return c.SendStatus(http.StatusInternalServerError)
      }

      return c.SendStatus(http.StatusNoContent)
   })

	log.Fatal(app.Listen(listenAddr))
}
