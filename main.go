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
		cluster := c.Params("cluster")
		if cluster == "" {
			logger.Error("empty cluster for node list")
			return c.SendStatus(http.StatusBadRequest)
		}

      list, err := nodeDB.List(cluster)
      if len(list) < 1 {
			logger.Warn("cluster not found",
				zap.String("cluster", cluster),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusNotFound)
      }

		logger.Info("listing cluster nodes",
			zap.String("cluster", c.Params("cluster", "")),
			zap.Int("count", len(list)),
		)

      return c.JSON(list)

   })

	app.Get("/:cluster/:node", func(c *fiber.Ctx) error {
		cluster := c.Params("cluster", "")
		if cluster == "" {
			logger.Error("empty cluster for node get")
			return c.SendStatus(http.StatusBadRequest)
		}

		node := c.Params("node", "")
		if node == "" {
			logger.Error("empty node for node get",
				zap.String("cluster", c.Params("cluster", "")),
			)
			return c.SendStatus(http.StatusBadRequest)
		}

      n, err := nodeDB.Get(cluster, node)
      if err != nil {
			logger.Warn("node not found",
				zap.String("cluster", cluster),
				zap.String("node", node),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusNotFound)
      }

		logger.Error("returning cluster node",
			zap.String("cluster", c.Params("cluster", "")),
			zap.String("node", n.ID),
			zap.String("ip", n.IP.String()),
			zap.Strings("endpoints", func() (out []string) {
				for _, ep := range n.KnownEndpoints {
					if !ep.Endpoint.IsZero() {
						out = append(out, ep.Endpoint.String())
					}
				}
				return out
			}()),
			zap.Error(err),
		)

      return c.JSON(n)
   })

	app.Put("/:cluster/:node", func(c *fiber.Ctx) error {
		var knownEndpoints []*types.KnownEndpoint

		if err := c.BodyParser(&knownEndpoints); err != nil {
			logger.Error("failed to parse node PUT",
				zap.String("cluster", c.Params("cluster", "")),
				zap.String("node", c.Params("node", "")),
				zap.Error(err),
			)
			return c.SendStatus(http.StatusBadRequest)
		}

		node := c.Params("node", "")
		if node == "" {
			logger.Error("invalid node key",
				zap.String("cluster", c.Params("cluster", "")),
				zap.String("node", c.Params("node", "")),
				zap.Error(err),
			)
		}

      if err := nodeDB.AddKnownEndpoints(c.Params("cluster", ""), node, knownEndpoints...); err != nil {
			logger.Error("failed to add known endpoints",
				zap.String("cluster", c.Params("cluster", "")),
				zap.String("node", node),
				zap.Strings("endpoints", func() (out []string) {
					for _, ep := range knownEndpoints {
						if !ep.Endpoint.IsZero() {
							out = append(out, ep.Endpoint.String())
						}
					}
					return out
				}()),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusInternalServerError)
		}

      return c.SendStatus(http.StatusNoContent)
	})

   app.Post("/:cluster", func(c *fiber.Ctx) error {
      n := new(types.Node)

      if err := c.BodyParser(n); err != nil {
			logger.Error("failed to parse node POST",
				zap.String("cluster", c.Params("cluster", "")),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusBadRequest)
      }

      if err := nodeDB.Add(c.Params("cluster", ""), n); err != nil {
			logger.Error("failed to add/update node",
				zap.String("cluster", c.Params("cluster", "")),
				zap.String("node", n.ID),
				zap.String("ip", n.IP.String()),
				zap.Strings("endpoints", func() (out []string) {
					for _, ep := range n.KnownEndpoints {
						if !ep.Endpoint.IsZero() {
							out = append(out, ep.Endpoint.String())
						}
					}
					return out
				}()),
				zap.Error(err),
			)
         return c.SendStatus(http.StatusInternalServerError)
      }

		logger.Info("add/update node",
				zap.String("cluster", c.Params("cluster", "")),
				zap.String("node", n.ID),
				zap.String("ip", n.IP.String()),
				zap.Strings("endpoints", func() (out []string) {
					for _, ep := range n.KnownEndpoints {
						if !ep.Endpoint.IsZero() {
							out = append(out, ep.Endpoint.String())
						}
					}
					return out
				}()),
		)

      return c.SendStatus(http.StatusNoContent)
   })

	logger.Fatal("listen exited",
		zap.Error(app.Listen(listenAddr)),
	)
}
