import { validateSVG } from './validate-svg'

describe('validateSVG', () => {
  it('should return true for valid SVG without script or onload', () => {
    const validSVG = '<svg><circle cx="50" cy="50" r="40" /></svg>'
    expect(validateSVG(validSVG)).toBe(true)
  })

  it('should return false for SVG with script tag', () => {
    const invalidSVGWithScript =
      '<svg><script>alert("malicious code")</script></svg>'
    expect(validateSVG(invalidSVGWithScript)).toBe(false)
  })

  it('should return false for SVG with onload attribute', () => {
    const invalidSVGWithOnload =
      '<svg onload="alert(\'malicious code\')"><circle cx="50" cy="50" r="40" /></svg>'
    expect(validateSVG(invalidSVGWithOnload)).toBe(false)
  })

  it('should return false for SVG with both script tag and onload attribute', () => {
    const invalidSVGWithBoth =
      '<svg onload="alert(\'malicious code\')"><script>alert("malicious code")</script></svg>'
    expect(validateSVG(invalidSVGWithBoth)).toBe(false)
  })

  it('should return true for SVG with harmless attributes', () => {
    const validSVGWithAttributes =
      '<svg width="100" height="100"><circle cx="50" cy="50" r="40" /></svg>'
    expect(validateSVG(validSVGWithAttributes)).toBe(true)
  })
})
