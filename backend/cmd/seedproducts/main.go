// cmd/seedproducts is a bulk-insert CLI for the ready-made products
// catalogue. There's no admin UI for product management yet, so this —
// reading a JSON file and inserting each entry — is the current way
// inventory gets into the products table at all.
//
// Usage:
//
//	export DATABASE_URL=... JWT_SECRET=...   # JWT_SECRET unused here but
//	                                          # required by config.Load
//	go run ./cmd/seedproducts -file seed/products.json
//
// JSON shape: an array of objects matching seedProduct below. See
// seed/products.json for real examples. image_url should be a path your
// frontend actually serves — e.g. "/images/products/ring-001.jpg" for
// an image placed in the Next.js app's public/images/products/
// directory (self-hosted, not a hotlinked third-party URL — see the
// frontend README for why). Leave it empty/omitted for a product with
// no photo yet; the frontend falls back to a placeholder.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"gemstore/internal/config"
	"gemstore/internal/database"
	"gemstore/internal/models"
	"gemstore/internal/repository"
)

type seedProduct struct {
	SKU           string  `json:"sku"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	Gender        string  `json:"gender"`
	MetalType     string  `json:"metal_type"`
	GemstoneType  string  `json:"gemstone_type"`
	BasePrice     float64 `json:"base_price"`
	StockQuantity int     `json:"stock_quantity"`
	ImageURL      string  `json:"image_url"`
}

func main() {
	filePath := flag.String("file", "seed/products.json", "path to a JSON array of products to insert")
	flag.Parse()

	raw, err := os.ReadFile(*filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", *filePath, err)
		os.Exit(1)
	}

	var entries []seedProduct
	if err := json.Unmarshal(raw, &entries); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", *filePath, err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "no products found in file — nothing to do")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer pool.Close()

	repo := repository.NewProductRepository(pool)

	created, skipped := 0, 0
	for _, e := range entries {
		product := models.Product{
			SKU:           e.SKU,
			Name:          e.Name,
			Category:      models.JewelryCategory(e.Category),
			Gender:        models.Gender(e.Gender),
			BasePrice:     e.BasePrice,
			StockQuantity: e.StockQuantity,
			IsActive:      true,
		}
		if e.Description != "" {
			product.Description = &e.Description
		}
		if e.MetalType != "" {
			product.MetalType = &e.MetalType
		}
		if e.GemstoneType != "" {
			product.GemstoneType = &e.GemstoneType
		}
		if e.ImageURL != "" {
			product.ImageURL = &e.ImageURL
		}

		if !product.Category.Valid() {
			fmt.Fprintf(os.Stderr, "skipping %s: invalid category %q\n", e.SKU, e.Category)
			skipped++
			continue
		}

		_, err := repo.Create(ctx, product)
		if err != nil {
			if errors.Is(err, repository.ErrSKUTaken) {
				fmt.Printf("skipping %s: already exists\n", e.SKU)
			} else {
				fmt.Fprintf(os.Stderr, "error creating %s: %v\n", e.SKU, err)
			}
			skipped++
			continue
		}
		fmt.Printf("created %s: %s\n", e.SKU, e.Name)
		created++
	}

	fmt.Printf("\ndone: %d created, %d skipped\n", created, skipped)
}
