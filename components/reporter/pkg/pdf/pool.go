// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pdf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	cn "github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

//go:generate mockgen --destination=pool.mock.go --package=pdf --copyright_file=../../COPYRIGHT . PDFGenerator

// Compile-time interface satisfaction check.
var _ PDFGenerator = (*WorkerPool)(nil)

// PDFGenerator defines the interface for submitting PDF generation tasks.
type PDFGenerator interface {
	// Submit sends an HTML string to the pool for PDF generation and blocks until completion.
	Submit(html, filename string) error
}

// Task represents a task to generate a PDF.
type Task struct {
	HTML     string
	Filename string
	Result   chan error
}

// WorkerPool manager multiple Chrome workers to generate PDFs.
type WorkerPool struct {
	tasks   chan Task
	wg      *sync.WaitGroup
	workers int
	timeout time.Duration
	logger  log.Logger
}

func blockedExternalURLPatterns() []string {
	return []string{
		"http://*",
		"https://*",
		"ws://*",
		"wss://*",
		"ftp://*",
		"file://*",
	}
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(num int, timeout time.Duration, logger log.Logger) *WorkerPool {
	wp := &WorkerPool{
		tasks:   make(chan Task),
		wg:      &sync.WaitGroup{},
		workers: num,
		timeout: timeout,
		logger:  logger,
	}
	for i := 0; i < num; i++ {
		wp.wg.Add(1)

		go func(workerID int) {
			defer func() {
				if r := recover(); r != nil {
					wp.logger.Log(context.Background(), log.LevelError, "Panic recovered in PDF worker", log.Int("worker_id", workerID), log.Any("panic", r), log.String("stack", string(debug.Stack())))
				}
			}()

			wp.startWorker(workerID)
		}(i)
	}

	return wp
}

// startWorker runs a Chrome worker to generate PDFs.
// Creates a single browser process per worker and reuses it for all tasks.
func (wp *WorkerPool) startWorker(_ int) {
	defer wp.wg.Done()

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), wp.getChromeOptions()...)
	defer allocCancel()

	for task := range wp.tasks {
		wp.processTask(allocCtx, task)
	}
}

// getChromeOptions returns optimized Chrome flags for PDF generation in containers with memory limits.
func (wp *WorkerPool) getChromeOptions() []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-plugins", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-features", "TranslateUI,site-per-process"),

		chromedp.Flag("max-old-space-size", cn.PDFChromeMaxOldSpaceSize),
		chromedp.Flag("js-flags", "--max-old-space-size="+cn.PDFChromeMaxOldSpaceSize),
		chromedp.Flag("disable-software-rasterizer", true),

		chromedp.Flag("force-fieldtrials", "OmniboxBundledExperimentV1/Disabled"),
	}
}

// processTask handles a single PDF generation task.
func (wp *WorkerPool) processTask(allocCtx context.Context, task Task) {
	htmlSizeKB := float64(len(task.HTML)) / cn.PDFBytesPerKB
	wp.logger.Log(allocCtx, log.LevelInfo, "Starting PDF generation for task", log.String("filename", task.Filename), log.Any("html_size_kb", htmlSizeKB), log.Any("timeout", wp.timeout))

	if len(task.HTML) > cn.PDFLargeHTMLThreshold {
		wp.logger.Log(allocCtx, log.LevelWarn, "Large HTML detected. Consider increasing PDF_TIMEOUT_SECONDS if timeouts occur", log.Any("html_size_kb", htmlSizeKB))
	}

	ctx, ctxCancel := chromedp.NewContext(allocCtx)
	defer ctxCancel()

	ctxTimeout, cancelTimeout := context.WithTimeout(ctx, wp.timeout)
	defer cancelTimeout()

	pdfBuf, err := wp.generatePDF(ctxTimeout, task.HTML)

	err = wp.processPDFResult(pdfBuf, task.Filename, err)

	task.Result <- err
}

// generatePDF renders HTML to PDF using Chrome headless.
//
// Security layers (defense in depth):
//  1. Origin isolation: navigates to about:blank and injects HTML via CDP
//     page.SetDocumentContent — gives the page an opaque origin, blocking
//     file:// access (meta refresh, iframes, img src).
//  2. Fetch interception: uses CDP Fetch domain to intercept ALL network
//     requests (including top-level navigations). Only about:blank is allowed;
//     every other URL (http://, https://, file://, data:, javascript:) is
//     rejected with network error. This blocks meta refresh to external URLs,
//     SSRF via sub-resources, and data: URI injection.
//  3. URL blocklist: network.SetBlockedURLs as a fallback layer for
//     sub-resource requests.
func (wp *WorkerPool) generatePDF(ctx context.Context, html string) ([]byte, error) {
	wp.logger.Log(ctx, log.LevelInfo, "Generating PDF via about:blank origin (sandboxed)")

	var pdfBuf []byte

	err := chromedp.Run(ctx,
		// Layer 3: block sub-resource URLs by pattern (fallback).
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}

			return network.SetBlockedURLs(blockedExternalURLPatterns()).Do(ctx)
		}),
		// Layer 2: intercept ALL requests via Fetch domain.
		// Any request not to about:blank is failed with AccessDenied.
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := fetch.Enable().WithPatterns([]*fetch.RequestPattern{
				{URLPattern: "*"},
			}).Do(ctx); err != nil {
				return fmt.Errorf("failed to enable fetch interception: %w", err)
			}

			chromedp.ListenTarget(ctx, func(ev any) {
				if req, ok := ev.(*fetch.EventRequestPaused); ok {
					go wp.handleInterceptedRequest(ctx, req)
				}
			})

			return nil
		}),
		// Layer 1: load content via about:blank origin.
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Disable fetch interception before injecting HTML.
			// The HTML itself is set via CDP (not a network request), so
			// interception is not needed after navigation. Disabling it
			// allows inline resources (data: images in CSS, etc.) to load
			// while still having blocked any navigation attempts.
			if err := fetch.Disable().Do(ctx); err != nil {
				wp.logger.Log(ctx, log.LevelWarn, "Failed to disable fetch interception", log.Err(err))
			}

			frameTree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to get frame tree: %w", err)
			}

			return page.SetDocumentContent(frameTree.Frame.ID, html).Do(ctx)
		}),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(cn.PDFRenderSettleDelay),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error

			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(cn.PDFPaperWidthInches).
				WithPaperHeight(cn.PDFPaperHeightInches).
				WithMarginTop(cn.PDFMarginInches).
				WithMarginBottom(cn.PDFMarginInches).
				WithMarginLeft(cn.PDFMarginInches).
				WithMarginRight(cn.PDFMarginInches).
				WithDisplayHeaderFooter(false).
				Do(ctx)

			return err
		}),
	)
	if err != nil {
		wp.logPDFGenerationError(ctx, err)
		return nil, err
	}

	return pdfBuf, nil
}

// handleInterceptedRequest decides whether to allow or block an intercepted request.
// Only about:blank navigation is allowed; all other URLs are blocked.
func (wp *WorkerPool) handleInterceptedRequest(ctx context.Context, req *fetch.EventRequestPaused) {
	url := req.Request.URL

	if url == "about:blank" {
		if err := fetch.ContinueRequest(req.RequestID).Do(ctx); err != nil {
			wp.logger.Log(ctx, log.LevelWarn, "Failed to continue about:blank request", log.Err(err))
		}

		return
	}

	wp.logger.Log(ctx, log.LevelWarn, "Blocked request from rendered HTML",
		log.String("url", url),
		log.String("resource_type", req.ResourceType.String()),
	)

	if err := fetch.FailRequest(req.RequestID, network.ErrorReasonAccessDenied).Do(ctx); err != nil {
		wp.logger.Log(ctx, log.LevelWarn, "Failed to block request", log.String("url", url), log.Err(err))
	}
}

// processPDFResult validates and writes the generated PDF to disk.
func (wp *WorkerPool) processPDFResult(pdfBuf []byte, filename string, err error) error {
	if err != nil {
		return err
	}

	if len(pdfBuf) < cn.PDFMinValidSizeBytes {
		wp.logger.Log(context.Background(), log.LevelError, "Final PDF too small", log.Int("bytes", len(pdfBuf)))
		return fmt.Errorf("generated PDF is too small (%d bytes), likely empty", len(pdfBuf))
	}

	if err := os.WriteFile(filename, pdfBuf, cn.PDFFilePermissions); err != nil {
		wp.logger.Log(context.Background(), log.LevelError, "Failed to write PDF file", log.Err(err))
		return err
	}

	wp.logger.Log(context.Background(), log.LevelInfo, "PDF generated successfully", log.Int("bytes", len(pdfBuf)), log.String("filename", filename))

	return nil
}

// logPDFGenerationError logs PDF generation errors with appropriate context.
func (wp *WorkerPool) logPDFGenerationError(ctx context.Context, err error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		wp.logger.Log(ctx, log.LevelError, "PDF generation timeout", log.Any("configured_timeout", wp.timeout), log.Err(err))
	} else if errors.Is(ctx.Err(), context.Canceled) {
		wp.logger.Log(ctx, log.LevelError, "PDF generation context canceled", log.Err(err))
	} else {
		wp.logger.Log(ctx, log.LevelError, "PDF generation failed", log.Err(err))
	}
}

// Submit sends a task to the pool and blocks until it is completed.
func (wp *WorkerPool) Submit(html, filename string) error {
	res := make(chan error, 1)
	wp.tasks <- Task{HTML: html, Filename: filename, Result: res}

	return <-res
}

// Close closes the pool and waits for all workers to finish.
func (wp *WorkerPool) Close() {
	close(wp.tasks)
	wp.wg.Wait()
}

// GetStats returns pool statistics
func (wp *WorkerPool) GetStats() map[string]any {
	return map[string]any{
		"workers":       wp.workers,
		"timeout":       wp.timeout,
		"tasks_pending": len(wp.tasks),
	}
}

// IsHealthy returns true if the pool is healthy
func (wp *WorkerPool) IsHealthy() bool {
	return wp.workers > 0 && wp.timeout > 0
}
