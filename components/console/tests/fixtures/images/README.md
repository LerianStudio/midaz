# Test Images

This folder contains images used for E2E testing.

## Available Test Images

### `test-avatar.png`
- **Size:** 100x100 pixels (~500 bytes)
- **Format:** PNG
- **Description:** Blue square image for testing avatar uploads
- **Used in:**
  - `onboarding-flow.spec.ts` - Theme configuration step
  - Any test requiring avatar/image upload

## Adding New Test Images

To add a new test image:

1. Place the image file in this directory
2. Use a descriptive name (e.g., `test-logo.jpg`, `test-banner.png`)
3. Keep file sizes small (<100KB) for faster test execution
4. Update this README with the image details

## Generating Test Images

You can regenerate the test images by running:

```bash
node generate-test-image.js
```

## Best Practices

- Keep images small (< 100KB)
- Use common formats (PNG, JPG, WebP)
- Don't commit large or unnecessary images
- Document each image's purpose in this README
