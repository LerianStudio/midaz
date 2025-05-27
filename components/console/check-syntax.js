const fs = require('fs')
const path = require('path')

const filePath = path.join(
  __dirname,
  'src/app/(routes)/plugins/reconciliation/processes/[id]/monitoring/page.tsx'
)
const content = fs.readFileSync(filePath, 'utf8')

// Count brackets
let openBrackets = 0
let closeBrackets = 0
let openParens = 0
let closeParens = 0
let openCurly = 0
let closeCurly = 0

for (let i = 0; i < content.length; i++) {
  const char = content[i]
  if (char === '[') openBrackets++
  if (char === ']') closeBrackets++
  if (char === '(') openParens++
  if (char === ')') closeParens++
  if (char === '{') openCurly++
  if (char === '}') closeCurly++
}

console.log('Bracket counts:')
console.log(
  '[ ]:',
  openBrackets,
  closeBrackets,
  openBrackets === closeBrackets ? '✓' : '✗'
)
console.log(
  '( ):',
  openParens,
  closeParens,
  openParens === closeParens ? '✓' : '✗'
)
console.log('{ }:', openCurly, closeCurly, openCurly === closeCurly ? '✓' : '✗')

// Find line 193-197
const lines = content.split('\n')
console.log('\nLines around error:')
for (let i = 190; i <= 200 && i < lines.length; i++) {
  console.log(`${i}: ${lines[i]}`)
}
