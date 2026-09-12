package db

import (
	"errors"
	"testing"
)

func TestInitSchemaIsIdempotentAndCreatesDefaultTemplate(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	templates, err := store.ListTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 || templates[0].Name != "Default" || !templates[0].IsDefault {
		t.Fatalf("templates = %#v, want one default template", templates)
	}
}

func TestUpdateTemplatePersistsEditableFields(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.CreateTemplate(ctx, Template{
		Name:        "Kids",
		Description: "old",
		PolicyJSON:  `{"IsAdministrator":false}`,
	}); err != nil {
		t.Fatal(err)
	}
	templates, err := store.ListTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	for _, template := range templates {
		if template.Name == "Kids" {
			id = template.ID
		}
	}
	if id == 0 {
		t.Fatalf("created template not found: %#v", templates)
	}
	if err := store.UpdateTemplate(ctx, Template{
		ID:          id,
		Name:        "Family",
		Description: "edited",
		PolicyJSON:  `{"IsAdministrator":false,"EnableAllFolders":true}`,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.Template(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Family" || got.Description != "edited" || got.PolicyJSON != `{"IsAdministrator":false,"EnableAllFolders":true}` {
		t.Fatalf("updated template = %#v", got)
	}
	err = store.UpdateTemplate(ctx, Template{
		ID:         999,
		Name:       "Missing",
		PolicyJSON: TemplatePolicyDefaultJSON,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing update error = %v, want ErrNotFound", err)
	}
}

func TestTemplateDefaultAndDeleteRules(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.CreateTemplate(ctx, Template{
		Name:       "Second",
		PolicyJSON: `{"IsAdministrator":false}`,
	}); err != nil {
		t.Fatal(err)
	}
	templates, err := store.ListTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var defaultID, secondID int64
	for _, template := range templates {
		if template.IsDefault {
			defaultID = template.ID
		}
		if template.Name == "Second" {
			secondID = template.ID
		}
	}
	if defaultID == 0 || secondID == 0 {
		t.Fatalf("templates = %#v, want default and second template", templates)
	}
	if err := store.DeleteTemplate(ctx, defaultID); !errors.Is(err, ErrTemplateIsDefault) {
		t.Fatalf("delete default error = %v, want ErrTemplateIsDefault", err)
	}
	if err := store.SetDefaultTemplate(ctx, secondID); err != nil {
		t.Fatal(err)
	}
	templates, err = store.ListTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if templates[0].ID != secondID || !templates[0].IsDefault {
		t.Fatalf("templates after default = %#v, want second first/default", templates)
	}
	if _, err := store.CreateInvite(ctx, Invite{
		TokenHash:  "hash-second",
		Label:      "invite",
		TemplateID: secondID,
		MaxUses:    1, BindingID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTemplate(ctx, secondID); !errors.Is(err, ErrTemplateIsDefault) {
		t.Fatalf("delete current default error = %v, want ErrTemplateIsDefault", err)
	}
	if err := store.SetDefaultTemplate(ctx, defaultID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTemplate(ctx, secondID); !errors.Is(err, ErrTemplateInUse) {
		t.Fatalf("delete in-use template error = %v, want ErrTemplateInUse", err)
	}
	if err := store.CreateTemplate(ctx, Template{
		Name:       "Unused",
		PolicyJSON: `{"IsAdministrator":false}`,
	}); err != nil {
		t.Fatal(err)
	}
	templates, err = store.ListTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var unusedID int64
	for _, template := range templates {
		if template.Name == "Unused" {
			unusedID = template.ID
		}
	}
	if err := store.DeleteTemplate(ctx, unusedID); err != nil {
		t.Fatalf("delete unused template error = %v", err)
	}
}
