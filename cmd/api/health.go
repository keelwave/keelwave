package main

import "net/http"

// HealthCheck godoc
//
//	@Summary		Health check
//	@Description	Returns service status, version, and environment.
//	@Tags			meta
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/health [get]
func (app *application) healthHandler(w http.ResponseWriter, r *http.Request) {
	data := map[string]string{
		"status":  "ok",
		"service": "keelwave",
		"version": version,
		"env":     app.config.env,
	}
	if err := app.jsonResponse(w, http.StatusOK, data); err != nil {
		app.internalServerError(w, r, err)
	}
}
