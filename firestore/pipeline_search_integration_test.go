// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/genproto/googleapis/type/latlng"
)

var restaurantDocs = map[string]map[string]interface{}{
	"sunnySideUp": {
		"name":                     "The Sunny Side Up",
		"description":              "A cozy neighborhood diner serving classic breakfast favorites all day long, from fluffy pancakes to savory omelets.",
		"location":                 &latlng.LatLng{Latitude: 39.7541, Longitude: -105.0002},
		"menu":                     "<h3>Breakfast Classics</h3><ul><li>Denver Omelet - $12</li><li>Buttermilk Pancakes - $10</li><li>Steak and Eggs - $16</li></ul><h3>Sides</h3><ul><li>Hash Browns - $4</li><li>Thick-cut Bacon - $5</li><li>Drip Coffee - $2</li></ul>",
		"average_price_per_person": 15,
	},
	"goldenWaffle": {
		"name":                     "The Golden Waffle",
		"description":              "Specializing exclusively in Belgian-style waffles. Open daily from 6:00 AM to 11:00 AM.",
		"location":                 &latlng.LatLng{Latitude: 39.7183, Longitude: -104.9621},
		"menu":                     "<h3>Signature Waffles</h3><ul><li>Strawberry Delight - $11</li><li>Chicken and Waffles - $14</li><li>Chocolate Chip Crunch - $10</li></ul><h3>Drinks</h3><ul><li>Fresh OJ - $4</li><li>Artisan Coffee - $3</li></ul>",
		"average_price_per_person": 13,
	},
	"lotusBlossomThai": {
		"name":                     "Lotus Blossom Thai",
		"description":              "Authentic Thai cuisine featuring hand-crushed spices and traditional family recipes from the Chiang Mai region.",
		"location":                 &latlng.LatLng{Latitude: 39.7315, Longitude: -104.9847},
		"menu":                     "<h3>Appetizers</h3><ul><li>Spring Rolls - $7</li><li>Chicken Satay - $9</li></ul><h3>Main Course</h3><ul><li>Pad Thai - $15</li><li>Green Curry - $16</li><li>Drunken Noodles - $15</li></ul>",
		"average_price_per_person": 22,
	},
	"mileHighCatch": {
		"name":                     "Mile High Catch",
		"description":              "Freshly sourced seafood offering a wide variety of Pacific fish and Atlantic shellfish in an upscale atmosphere.",
		"location":                 &latlng.LatLng{Latitude: 39.7401, Longitude: -104.9903},
		"menu":                     "<h3>From the Raw Bar</h3><ul><li>Oysters (Half Dozen) - $18</li><li>Lobster Cocktail - $22</li></ul><h3>Entrees</h3><ul><li>Pan-Seared Salmon - $28</li><li>King Crab Legs - $45</li><li>Fish and Chips - $19</li></ul>",
		"average_price_per_person": 45,
	},
	"peakBurgers": {
		"name":                     "Peak Burgers",
		"description":              "Casual burger joint focused on locally sourced Colorado beef and hand-cut fries.",
		"location":                 &latlng.LatLng{Latitude: 39.7622, Longitude: -105.0125},
		"menu":                     "<h3>Burgers</h3><ul><li>The Peak Double - $12</li><li>Bison Burger - $15</li><li>Veggie Stack - $11</li></ul><h3>Sides</h3><ul><li>Truffle Fries - $6</li><li>Onion Rings - $5</li></ul>",
		"average_price_per_person": 18,
	},
	"solTacos": {
		"name":                     "El Sol Tacos",
		"description":              "A vibrant street-side taco stand serving up quick, delicious, and traditional Mexican street food.",
		"location":                 &latlng.LatLng{Latitude: 39.6952, Longitude: -105.0274},
		"menu":                     "<h3>Tacos ($3.50 each)</h3><ul><li>Al Pastor</li><li>Carne Asada</li><li>Pollo Asado</li><li>Nopales (Cactus)</li></ul><h3>Beverages</h3><ul><li>Horchata - $4</li><li>Mexican Coke - $3</li></ul>",
		"average_price_per_person": 12,
	},
	"eastsideTacos": {
		"name":                     "Eastside Cantina",
		"description":              "Authentic street tacos and hand-shaken margaritas on the vibrant east side of the city.",
		"location":                 &latlng.LatLng{Latitude: 39.735, Longitude: -104.885},
		"menu":                     "<h3>Tacos</h3><ul><li>Carnitas Tacos - $4</li><li>Barbacoa Tacos - $4.50</li><li>Shrimp Tacos - $5</li></ul><h3>Drinks</h3><ul><li>House Margarita - $9</li><li>Jarritos - $3</li></ul>",
		"average_price_per_person": 18,
	},
	"eastsideChicken": {
		"name":                     "Eastside Chicken",
		"description":              "Fried chicken to go - next to Eastside Cantina.",
		"location":                 &latlng.LatLng{Latitude: 39.735, Longitude: -104.885},
		"menu":                     "<h3>Fried Chicken</h3><ul><li>Drumstick - $4</li><li>Wings - $1</li><li>Sandwich - $9</li></ul><h3>Drinks</h3><ul><li>House Margarita - $9</li><li>Jarritos - $3</li></ul>",
		"average_price_per_person": 12,
	},
}

func assertResultIds(t *testing.T, snapshot *PipelineSnapshot, ids ...string) {
	t.Helper()
	var resultIds []string
	results, err := snapshot.Results().GetAll()
	if err != nil {
		t.Fatalf("Failed to get results: %v", err)
	}
	for _, res := range results {
		if res.Ref() != nil {
			resultIds = append(resultIds, res.Ref().ID)
		}
	}
	if diff := cmp.Diff(ids, resultIds); diff != "" {
		t.Errorf("Result IDs mismatch (-want +got):\n%s", diff)
	}
}

func TestIntegration_PipelineSearch(t *testing.T) {
	skipIfEdition(t, "Pipeline queries", editionStandard)
	if useEmulator {
		t.Skip("Search queries are not supported in the emulator.")
	}

	ctx := context.Background()
	client := integrationClient(t)

	collectionName := "TextSearchIntegrationTests"
	coll := client.Collection(collectionName)

	// Setup restaurant docs
	// A batch will be used to update the test collection to the desired state
	batch := client.Batch()
	for k, v := range restaurantDocs {
		batch.Set(coll.Doc(k), v)
	}
	_, err := batch.Commit(ctx)
	if err != nil {
		t.Fatalf("Failed to setup restaurant docs: %v", err)
	}
	t.Cleanup(func() {
		deleteCollection(ctx, coll)
	})

	t.Run("searchFullDocument", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(collectionName).Search(WithSearchQuery("waffles"))
		snapshot := pipeline.Execute(ctx)
		assertResultIds(t, snapshot, "goldenWaffle")
	})

	t.Run("geoNearQuery", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(collectionName).Search(
			WithSearchQuery(
				FieldOf("location").
					GeoDistance(&latlng.LatLng{Latitude: 39.6985, Longitude: -105.024}).
					LessThanOrEqual(1000)),
		)
		snapshot := pipeline.Execute(ctx)
		assertResultIds(t, snapshot, "solTacos")
	})

	t.Run("negateMatch", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(collectionName).
			Search(WithSearchQuery(DocumentMatches("coffee -waffles")))
		snapshot := pipeline.Execute(ctx)
		assertResultIds(t, snapshot, "sunnySideUp")
	})

	t.Run("rqueryAsQueryParam", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(collectionName).
			Search(WithSearchQuery("chicken wings"))
		snapshot := pipeline.Execute(ctx)
		assertResultIds(t, snapshot, "eastsideChicken")
	})

	// add fields
	t.Run("addFields_score", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(collectionName).
			Search(
				WithSearchQuery(DocumentMatches("waffles")),
				WithSearchAddFields(Score().As("searchScore")),
			).
			Select([]any{"name", "searchScore"})

		snapshot := pipeline.Execute(ctx)
		results, err := snapshot.Results().GetAll()
		if err != nil {
			t.Fatalf("GetAll failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		result := results[0]
		data := result.Data()
		if data["name"] != "The Golden Waffle" {
			t.Errorf("Expected name 'The Golden Waffle', got %v", data["name"])
		}
		score, ok := data["searchScore"].(float64)
		if !ok || score <= 0.0 {
			t.Errorf("Expected searchScore > 0.0, got %v", data["searchScore"])
		}
	})

	// sort
	t.Run("sort_byScore", func(t *testing.T) {
		pipeline := client.Pipeline().Collection(collectionName).Search(
			WithSearchQuery(DocumentMatches("tacos")),
			WithSearchSort(Score().Descending()),
		)
		// // WithSearchQueryEnhancementDisabled(),
		snapshot := pipeline.Execute(ctx)
		assertResultIds(t, snapshot, "eastsideTacos", "solTacos")
	})

	t.Run("sort_byDistance", func(t *testing.T) {
		queryLocation := &latlng.LatLng{Latitude: 39.6985, Longitude: -105.024}
		pipeline := client.Pipeline().Collection(collectionName).Search(
			WithSearchQuery(
				FieldOf("location").
					GeoDistance(queryLocation).
					LessThanOrEqual(5600),
			),
			WithSearchSort(GeoDistance("location", queryLocation).Ascending()),
		)
		snapshot := pipeline.Execute(ctx)
		assertResultIds(t, snapshot, "solTacos", "lotusBlossomThai", "mileHighCatch")
	})
}
