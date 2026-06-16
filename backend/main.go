package main

import (
	"context"
	"karez-system/alert"
	"karez-system/config"
	"karez-system/db"
	"karez-system/handlers"
	"karez-system/models"
	"karez-system/mqtt"
	"karez-system/optimization"
	"karez-system/simulation"
	"log"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	database, err := db.New(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("Database connected successfully")

	mqttClient, err := mqtt.New(cfg)
	if err != nil {
		log.Printf("Warning: Failed to connect to MQTT broker: %v", err)
		log.Println("Continuing without MQTT support")
		mqttClient = nil
	} else {
		defer mqttClient.Close()
		log.Println("MQTT connected successfully")
	}

	simulator := simulation.New(database)
	allocator := optimization.New(database)
	alertMgr := alert.New(database, mqttClient, simulator, allocator)

	if mqttClient != nil {
		err := mqttClient.SubscribeSensorData(func(msg *mqtt.SensorMessage) {
			sensorData := &models.SensorData{
				Time:            msg.Time,
				KarezID:         msg.KarezID,
				SegmentID:       msg.SegmentID,
				ShaftID:         msg.ShaftID,
				SensorType:      msg.SensorType,
				SensorID:        msg.SensorID,
				FlowRate:        msg.FlowRate,
				WaterLevel:      msg.WaterLevel,
				ShaftWaterLevel: msg.ShaftWaterLevel,
				Evaporation:     msg.Evaporation,
				Temperature:     msg.Temperature,
				Turbidity:       msg.Turbidity,
				Velocity:        msg.Velocity,
			}

			ctx := context.Background()
			if err := database.InsertSensorData(ctx, sensorData); err != nil {
				log.Printf("Failed to insert sensor data from MQTT: %v", err)
			}
		})
		if err != nil {
			log.Printf("Warning: Failed to subscribe to sensor data: %v", err)
		}
	}

	h := handlers.New(database, simulator, allocator, alertMgr)

	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	api := r.Group("/api")
	{
		api.GET("/karez", h.GetKarezSystems)
		api.GET("/karez/:karez_id/segments", h.GetAqueductSegments)
		api.GET("/karez/:karez_id/shafts", h.GetVerticalShafts)
		api.GET("/karez/:karez_id/branches", h.GetBranchChannels)
		api.GET("/karez/:karez_id/oases", h.GetOases)
		api.GET("/karez/:karez_id/dashboard", h.GetDashboardData)

		api.POST("/sensor", h.PostSensorData)
		api.GET("/sensor/:karez_id/latest", h.GetLatestSensorData)
		api.GET("/sensor/:karez_id/range", h.GetSensorDataByRange)

		api.POST("/simulate", h.RunSimulation)
		api.POST("/simulate/hydraulic", h.SimulateHydraulic)

		api.POST("/allocate", h.RunAllocation)

		api.GET("/alerts/:karez_id", h.GetActiveAlerts)
		api.POST("/alerts/check/:karez_id", h.CheckAlerts)
		api.POST("/alerts/acknowledge", h.AcknowledgeAlert)
		api.POST("/alerts/resolve", h.ResolveAlert)
	}

	go startPeriodicTasks(database, simulator, alertMgr, cfg)

	log.Printf("Server starting on port %s", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func startPeriodicTasks(database *db.Database, simulator *simulation.HydraulicSimulator,
	alertMgr *alert.AlertManager, cfg *config.Config) {

	simTicker := time.NewTicker(time.Duration(cfg.SimulationInterval) * time.Second)
	alertTicker := time.NewTicker(time.Duration(cfg.AlertCheckInterval) * time.Second)

	defer simTicker.Stop()
	defer alertTicker.Stop()

	ctx := context.Background()

	for {
		select {
		case <-simTicker.C:
			systems, err := database.GetKarezSystems(ctx)
			if err != nil {
				log.Printf("Error getting karez systems: %v", err)
				continue
			}
			for _, sys := range systems {
				if err := simulator.RunFullSimulation(ctx, sys.ID); err != nil {
					log.Printf("Error running simulation for karez %d: %v", sys.ID, err)
				}
			}

		case <-alertTicker.C:
			if err := alertMgr.CheckAllKarez(ctx); err != nil {
				log.Printf("Error checking alerts: %v", err)
			}
		}
	}
}
