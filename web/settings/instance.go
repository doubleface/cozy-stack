// Package settings regroups some API methods to facilitate the usage of the
// io.cozy settings documents. For example, it has a route for getting a CSS
// with some CSS variables that can be used as a theme.
package settings

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

type apiInstance struct {
	doc *couchdb.JSONDoc
}

func (i *apiInstance) ID() string                             { return i.doc.ID() }
func (i *apiInstance) Rev() string                            { return i.doc.Rev() }
func (i *apiInstance) DocType() string                        { return consts.Settings }
func (i *apiInstance) Clone() couchdb.Doc                     { return i }
func (i *apiInstance) SetID(id string)                        { i.doc.SetID(id) }
func (i *apiInstance) SetRev(rev string)                      { i.doc.SetRev(rev) }
func (i *apiInstance) Relationships() jsonapi.RelationshipMap { return nil }
func (i *apiInstance) Included() []jsonapi.Object             { return nil }
func (i *apiInstance) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/instance"}
}
func (i *apiInstance) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.doc)
}

func getInstance(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(instance, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return err
	}
	doc.Type = consts.Settings
	doc.M["locale"] = instance.Locale

	if err = permissions.Allow(c, permissions.GET, doc); err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}

func updateInstance(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	doc := &couchdb.JSONDoc{}
	obj, err := jsonapi.Bind(c.Request(), doc)
	if err != nil {
		return err
	}

	doc.Type = consts.Settings
	doc.SetID(consts.InstanceSettingsID)
	doc.SetRev(obj.Meta.Rev)

	if err := permissions.Allow(c, permissions.PUT, doc); err != nil {
		return err
	}

	if locale, ok := doc.M["locale"].(string); ok {
		delete(doc.M, "locale")
		inst.Locale = locale
		if err := instance.Update(inst); err != nil {
			return err
		}
	}

	if err := couchdb.UpdateDoc(inst, doc); err != nil {
		return err
	}

	doc.M["locale"] = inst.Locale
	return jsonapi.Data(c, http.StatusOK, &apiInstance{doc}, nil)
}
