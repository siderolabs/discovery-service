package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/talos-systems/wglan-manager/db"
	"github.com/talos-systems/wglan-manager/types"
	"go.uber.org/zap"
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

	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalln("failed to initialise logger:", err)
	}

	if os.Getenv("MODE") == "dev" {
		devMode = true
		logger, err = zap.NewDevelopment()
		if err != nil {
			log.Fatalln("failed to initialise development logger:", err)
		}
	}

	defer logger.Sync()

   nodeDB = db.New()

   app := fiber.New()

	app.Get("/:cluster", func(c *fiber.Ctx) error {
		cluster := c.Get("cluster")
		if cluster == "" {
			return c.SendStatus(http.StatusBadRequest)
		}

      list, err := nodeDB.List(cluster)
      if len(list) < 1 {
			logger.Warn("cluster not found: ",
				zap.String("cluster", cluster),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusNotFound)
      }

      return c.JSON(list)

   })

	app.Get("/:cluster/:node", func(c *fiber.Ctx) error {
		cluster := c.Get("cluster", "")
		if cluster == "" {
			return c.SendStatus(http.StatusBadRequest)
		}

		node := c.Get("node", "")
		if node == "" {
			return c.SendStatus(http.StatusBadRequest)
		}

      list, err := nodeDB.List(cluster)
      if len(list) < 1 {
			logger.Warn("cluster empty",
				zap.String("cluster", cluster),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusNotFound)
      }

      return c.JSON(list)
   })

   app.Post("/:cluster", func(c *fiber.Ctx) error {
      n := new(types.Node)

      if err := c.BodyParser(n); err != nil {
         return c.SendStatus(http.StatusBadRequest)
      }

      if err := nodeDB.AddEndpoints(c.Get("cluster", ""), n.ID, n.KnownEndpoints); err != nil {
			logger.Error("failed to add endpoints to node",
				zap.String("cluster", c.Get("cluster", "")),
				zap.String("node", n.ID),
				zap.Strings("endpoints", n.KnownEndpoints),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusInternalServerError)
      }

      return c.SendStatus(http.StatusNoContent)
   })

	logger.Fatal("listen exited",
		zap.Error(app.Listen(listenAddr)),
	)
}
