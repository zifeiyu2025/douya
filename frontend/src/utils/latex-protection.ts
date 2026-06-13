// latex-protection.ts: LaTeX 预处理工具
// 移植自 llama.cpp 原生 webui (tools/ui/src/lib/utils/latex-protection.ts)
// 主要功能：
//   1. preprocessLaTeX: 保护代码块和 LaTeX 表达式，转义货币美元符号，转换分隔符
//   2. maskInlineLaTeX: 智能识别行内公式 vs 货币金额

// ===== 常量定义（移植自 llama.cpp constants/latex-protection.ts）=====

/** 匹配代码块（围栏和行内），用于排除 LaTeX 处理 */
const CODE_BLOCK_REGEXP = /(```[\s\S]*?```|`[^`\n]+`)/g

/**
 * 匹配 LaTeX 数学分隔符 \(...\) 和 \[...\]（仅未转义的），
 * 同时捕获代码块以便跳过。
 * group 1: 代码块
 * group 2: 方括号 \[...\]
 * group 3: 圆括号 \(...\)
 */
const LATEX_MATH_AND_CODE_PATTERN =
    /(```[\S\s]*?```|`.*?`)|(?<!\\)\\\[([\S\s]*?[^\\])\\]|(?<!\\)\\\((.*?)\\\)/g

/** 匹配 $$...\\...$$ 块（带换行的块级公式） */
const LATEX_LINEBREAK_REGEXP = /\$\$([\s\S]*?\\\\[\s\S]*?)\$\$/

/** mhchem 正则映射 */
const MHCHEM_PATTERN_MAP: readonly [RegExp, string][] = [
    [/(\s)\$\\ce{/g, '$1$\\\\ce{'],
    [/(\s)\$\\pu{/g, '$1$\\\\pu{'],
] as const

// ===== maskInlineLaTeX =====

/**
 * 智能识别行内 $...$ 公式 vs 货币金额
 * 公式 → <<LATEX_n>> 占位符，货币 → 保留原样
 *
 * 判断规则：
 * - $ 前有字母/数字/$/_/- → 不是公式（如 var$）
 * - $ 后紧跟数字且 $ 结尾后也有字母/数字 → 不是公式（如 $5.99）
 * - $$ 内容为空 → 不是公式
 * - 其他 → 视为公式
 */
export function maskInlineLaTeX(content: string, latexExpressions: string[]): string {
    if (!content.includes('$')) {
        return content
    }
    return content
        .split('\n')
        .map((line) => {
            if (line.indexOf('$') === -1) {
                return line
            }

            let processedLine = ''
            let currentPosition = 0

            while (currentPosition < line.length) {
                const openDollarIndex = line.indexOf('$', currentPosition)

                if (openDollarIndex === -1) {
                    processedLine += line.slice(currentPosition)
                    break
                }

                const closeDollarIndex = line.indexOf('$', openDollarIndex + 1)

                if (closeDollarIndex === -1) {
                    processedLine += line.slice(currentPosition)
                    break
                }

                const charBeforeOpen = openDollarIndex > 0 ? line[openDollarIndex - 1] : ''
                const charAfterOpen = line[openDollarIndex + 1]
                const charBeforeClose =
                    openDollarIndex + 1 < closeDollarIndex ? line[closeDollarIndex - 1] : ''
                const charAfterClose = closeDollarIndex + 1 < line.length ? line[closeDollarIndex + 1] : ''

                let shouldSkipAsNonLatex = false

                if (closeDollarIndex === currentPosition + 1) {
                    // $$ 内容为空
                    shouldSkipAsNonLatex = true
                }

                if (/[A-Za-z0-9_$-]/.test(charBeforeOpen)) {
                    // $ 前有字母/数字/$/_/-，不是公式
                    shouldSkipAsNonLatex = true
                }

                if (
                    /[0-9]/.test(charAfterOpen) &&
                    (/[A-Za-z0-9_$-]/.test(charAfterClose) || ' ' === charBeforeClose)
                ) {
                    // $ 后紧跟数字，看起来像金额
                    shouldSkipAsNonLatex = true
                }

                if (shouldSkipAsNonLatex) {
                    processedLine += line.slice(currentPosition, openDollarIndex + 1)
                    currentPosition = openDollarIndex + 1
                    continue
                }

                // 视为 LaTeX 公式
                processedLine += line.slice(currentPosition, openDollarIndex)
                const latexContent = line.slice(openDollarIndex, closeDollarIndex + 1)
                latexExpressions.push(latexContent)
                processedLine += `<<LATEX_${latexExpressions.length - 1}>>`
                currentPosition = closeDollarIndex + 1
            }

            return processedLine
        })
        .join('\n')
}

// ===== 内部辅助函数 =====

/** 转换 \(...\) → $...$，\[...\] → $$...$$（跳过代码块） */
function escapeBrackets(text: string): string {
    return text.replace(
        LATEX_MATH_AND_CODE_PATTERN,
        (
            match: string,
            codeBlock: string | undefined,
            squareBracket: string | undefined,
            roundBracket: string | undefined
        ): string => {
            if (codeBlock != null) {
                return codeBlock
            } else if (squareBracket != null) {
                return `$$${squareBracket}$$`
            } else if (roundBracket != null) {
                return `$${roundBracket}$`
            }
            return match
        }
    )
}

/** mhchem 转义 */
function escapeMhchem(text: string): string {
    return MHCHEM_PATTERN_MAP.reduce((result, [pattern, replacement]) => {
        return result.replace(pattern, replacement)
    }, text)
}

const doEscapeMhchem = false

// ===== preprocessLaTeX =====

/**
 * LaTeX 预处理：保护代码块和 LaTeX 表达式，转义货币美元符号，转换分隔符
 *
 * 处理流程（8 步）：
 * 0. 移除引用标记 (>)
 * 1. 保护代码块 → <<CODE_BLOCK_n>>
 * 2. 保护 LaTeX → <<LATEX_n>>
 * 3. 转义独立 $ → \$（货币符号）
 * 4. 恢复 LaTeX
 * 5. 转换 \(...\) → $...$，\[...\] → $$...$$
 * 6. 转换剩余的 \(...\) 和 \[...\]
 * 7. 恢复代码块
 * 8. 恢复引用标记
 */
export function preprocessLaTeX(content: string): string {
    // Step 0: 临时移除引用标记
    const blockquoteMarkers: Map<number, string> = new Map()
    const lines = content.split('\n')
    const processedLines = lines.map((line, index) => {
        const match = line.match(/^(>\s*)/)
        if (match) {
            blockquoteMarkers.set(index, match[1])
            return line.slice(match[1].length)
        }
        return line
    })
    content = processedLines.join('\n')

    // Step 1: 保护代码块
    const codeBlocks: string[] = []
    content = content.replace(CODE_BLOCK_REGEXP, (match) => {
        codeBlocks.push(match)
        return `<<CODE_BLOCK_${codeBlocks.length - 1}>>`
    })

    // Step 2: 保护 LaTeX 表达式
    const latexExpressions: string[] = []

    // 匹配 \S...\[...\] 并保护
    content = content.replace(/([\S].*?)\\\[([\s\S]*?)\\\](.*)/g, (match, group1, group2, group3) => {
        if (group1.endsWith('\\')) {
            return match
        }
        const hasSuffix = /\S/.test(group3)
        let optBreak: string

        if (hasSuffix) {
            latexExpressions.push(`\\(${group2.trim()}\\)`)
            optBreak = ''
        } else {
            latexExpressions.push(`\\[${group2}\\]`)
            optBreak = '\n'
        }

        return `${group1}${optBreak}<<LATEX_${latexExpressions.length - 1}>>${optBreak}${group3}`
    })

    // 匹配 \(...\), \[...\], $$...$$ 并保护
    content = content.replace(
        /(\$\$[\s\S]*?\$\$|(?<!\\)\\\[[\s\S]*?\\\]|(?<!\\)\\\(.*?\\\))/g,
        (match) => {
            latexExpressions.push(match)
            return `<<LATEX_${latexExpressions.length - 1}>>`
        }
    )

    // 保护行内 $...$（排除货币金额）
    content = maskInlineLaTeX(content, latexExpressions)

    // Step 3: 转义独立的 $（货币符号，如 $5 → \$5）
    content = content.replace(/\$(?=\d)/g, '\\$')

    // Step 4: 恢复 LaTeX 表达式
    content = content.replace(/<<LATEX_(\d+)>>/g, (_, index) => {
        let expr = latexExpressions[parseInt(index)]
        const match = expr.match(LATEX_LINEBREAK_REGEXP)
        if (match) {
            const formula = match[1]
            const prefix = formula.startsWith('\n') ? '' : '\n'
            const suffix = formula.endsWith('\n') ? '' : '\n'
            expr = '$$' + prefix + formula + suffix + '$$'
        }
        return expr
    })

    // Step 5: 转换括号分隔符（\(...\) → $...$，\[...\] → $$...$$）
    content = escapeBrackets(content)

    if (doEscapeMhchem && (content.includes('\\ce{') || content.includes('\\pu{'))) {
        content = escapeMhchem(content)
    }

    // Step 6: 转换剩余的 \(...\) → $...$，\[...\] → $$...$$
    content = content
        .replace(/(?<!\\)\\\((.+?)\\\)/g, '$$$1$')
        .replace(
            /(?<!\\)\\\[([\s\S]*?)\\\]/g,
            (_, c: string) => `$$${c}$$`
        )

    // Step 7: 恢复代码块
    content = content.replace(/<<CODE_BLOCK_(\d+)>>/g, (_, index) => {
        return codeBlocks[parseInt(index)]
    })

    // Step 8: 恢复引用标记
    if (blockquoteMarkers.size > 0) {
        const finalLines = content.split('\n')
        const restoredLines = finalLines.map((line, index) => {
            const marker = blockquoteMarkers.get(index)
            return marker ? marker + line : line
        })
        content = restoredLines.join('\n')
    }

    return content
}
