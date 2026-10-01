package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type StoreController struct{ store service.StoreService }

func NewStoreController(g *gin.RouterGroup) *StoreController {
	a := &StoreController{}
	g.GET("/state", a.state)
	g.POST("/settings", a.settings)
	g.POST("/plan", a.plan)
	return a
}

func (a *StoreController) state(c *gin.Context) {
	state, err := a.store.AdminState()
	jsonObj(c, state, err)
}

func (a *StoreController) settings(c *gin.Context) {
	var cfg model.StoreConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		jsonMsg(c, I18nWeb(c, "save"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "save"), a.store.UpdateStoreSettings(cfg))
}

func (a *StoreController) plan(c *gin.Context) {
	var plan model.StorePlan
	if err := c.ShouldBindJSON(&plan); err != nil {
		jsonMsg(c, I18nWeb(c, "save"), err)
		return
	}
	jsonMsg(c, I18nWeb(c, "save"), a.store.SavePlan(plan))
}
