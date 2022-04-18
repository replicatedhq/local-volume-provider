package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/replicatedhq/local-volume-provider/pkg/plugin"
	"github.com/replicatedhq/local-volume-provider/pkg/version"
)

func main() {
	if len(os.Args) > 0 && os.Args[1] == "version" {
		fmt.Println(version.Get())
		os.Exit(0)
	}

	app := fiber.New()

	mountPoint := os.Getenv("MOUNT_POINT")
	if mountPoint == "" {
		mountPoint = "/var/velero-local-volume-provider"
	}

	_, err := os.Stat(mountPoint)
	if err != nil {
		log.Fatalf("Could not find mountpoint: %s", mountPoint)
	}

	// livez endpoint
	app.Get("/livez", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	app.Use(logger.New())

	// signing guard middleware
	app.Use(func(c *fiber.Ctx) error {
		rawUrl := c.Request().URI().String()
		valid, err := plugin.IsSignedURLValid(rawUrl, os.Getenv("VELERO_NAMESPACE"))
		if err != nil {
			return c.SendStatus(http.StatusInternalServerError)
		}
		if !valid {
			return c.SendStatus(http.StatusBadRequest)
		}

		return c.Next()
	})

	// static file serving middleware
	app.Use(filesystem.New(filesystem.Config{
		Root: http.Dir(mountPoint),
	}))

	app.Listen(":3000")
}
