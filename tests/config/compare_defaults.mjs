// compare_defaults.mjs
// 契约比对脚本：Go DefaultConfig() ↔ TS DEFAULT_CONFIG
//
// 运行方式（需先通过 Go 测试导出 JSON）：
//   go test ./tests/config/... -run TestContractExportDefaultConfig -count=1
//   node tests/config/compare_defaults.mjs
//
// 当 Go 默认值变更时，本脚本会检测到差异并报错，提醒开发者同步修改 TS DEFAULT_CONFIG。

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const rootDir = path.resolve(__dirname, '..', '..');

// ============================================================
// 1. 读取 Go 测试导出的 go_default_config.json
// ============================================================
const goJsonPath = path.join(__dirname, 'go_default_config.json');
if (!fs.existsSync(goJsonPath)) {
    console.error(`[ERROR] 找不到 ${goJsonPath}`);
    console.error(`        请先运行: go test ./tests/config/... -run TestContractExportDefaultConfig -count=1`);
    process.exit(1);
}
const goConfig = JSON.parse(fs.readFileSync(goJsonPath, 'utf-8'));

// ============================================================
// 2. 从 chat.ts 提取 DEFAULT_CONFIG 对象文本
// ============================================================
const chatTsPath = path.join(rootDir, 'frontend', 'src', 'types', 'chat.ts');
const tsContent = fs.readFileSync(chatTsPath, 'utf-8');

const startMarker = 'export const DEFAULT_CONFIG: Config = {';
const startIdx = tsContent.indexOf(startMarker);
if (startIdx === -1) {
    console.error('[ERROR] 在 chat.ts 中找不到 DEFAULT_CONFIG 定义');
    process.exit(1);
}

const objStart = startIdx + startMarker.length;
// 找到匹配的闭合大括号（考虑嵌套）
let depth = 1;
let endIdx = objStart;
for (let i = objStart; i < tsContent.length; i++) {
    const ch = tsContent[i];
    if (ch === '{') depth++;
    else if (ch === '}') {
        depth--;
        if (depth === 0) { endIdx = i; break; }
    }
}
const configBody = tsContent.slice(objStart, endIdx);

// ============================================================
// 3. 解析 TS 字段（每行格式: key: value, // 注释）
// ============================================================
// 注意：字符串值中可能包含 "//"（如 URL 'http://...'），
// 必须优先匹配字符串字面量，避免误判为行尾注释。
const tsConfig = {};
const lines = configBody.split('\n');
for (const line of lines) {
    // 跳过空行和纯注释行
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('//')) continue;

    // 优先匹配字符串字面量：key: 'value', 或 key: "value", 或带尾随注释
    // 使用反向引用 \2 确保开闭引号一致
    const strMatch = trimmed.match(/^(\w+)\s*:\s*(['"])([\s\S]*?)\2\s*,?\s*(?:\/\/.*)?$/);
    if (strMatch) {
        tsConfig[strMatch[1]] = strMatch[3];
        continue;
    }

    // 非字符串类型：key: value, 或 key: value // comment
    const match = trimmed.match(/^(\w+)\s*:\s*(.+?)(?:,)?(?:\s*\/\/.*)?$/);
    if (!match) continue;

    const key = match[1];
    let valueStr = match[2].trim();
    // 去除行尾注释（如 "true // 与 Go DefaultConfig 对齐"）
    const commentIdx = valueStr.indexOf('//');
    if (commentIdx !== -1) {
        valueStr = valueStr.slice(0, commentIdx).trim();
    }
    // 去除尾随逗号
    if (valueStr.endsWith(',')) {
        valueStr = valueStr.slice(0, -1).trim();
    }

    // 解析值类型
    if (valueStr === 'null') {
        tsConfig[key] = null;
    } else if (valueStr === 'true') {
        tsConfig[key] = true;
    } else if (valueStr === 'false') {
        tsConfig[key] = false;
    } else if (!isNaN(Number(valueStr))) {
        tsConfig[key] = Number(valueStr);
    }
    // 其他类型（对象、数组等）忽略——DEFAULT_CONFIG 中应全部为标量
}

// ============================================================
// 4. 逐字段比对
// ============================================================
const goKeys = Object.keys(goConfig);
const tsKeys = Object.keys(tsConfig);

const missingInTs = goKeys.filter(k => !(k in tsConfig));
const missingInGo = tsKeys.filter(k => !(k in goConfig));

const mismatches = [];
for (const key of goKeys) {
    if (!(key in tsConfig)) continue;
    const goVal = goConfig[key];
    const tsVal = tsConfig[key];

    // 类型对齐规则：
    //   Go nil（*bool 未设置）→ TS null
    //   Go bool/true|false → TS boolean
    //   Go float64/int → TS number（JSON 中数字统一为 number）
    //   Go string → TS string
    if (goVal === null && tsVal === null) continue;
    if (goVal === tsVal) continue;
    // 数字近似比较（避免浮点精度问题）
    if (typeof goVal === 'number' && typeof tsVal === 'number') {
        if (Math.abs(goVal - tsVal) < 1e-9) continue;
    }
    mismatches.push({ key, goVal, tsVal });
}

// ============================================================
// 5. 输出比对结果
// ============================================================
console.log('='.repeat(60));
console.log('Go DefaultConfig() ↔ TS DEFAULT_CONFIG 契约比对');
console.log('='.repeat(60));
console.log(`Go 字段数: ${goKeys.length}`);
console.log(`TS 字段数: ${tsKeys.length}`);
console.log(`TS 缺失字段: ${missingInTs.length}`);
console.log(`Go 缺失字段: ${missingInGo.length}`);
console.log(`不一致字段: ${mismatches.length}`);
console.log('-'.repeat(60));

if (missingInTs.length > 0) {
    console.log('\n[WARN] TS DEFAULT_CONFIG 缺失字段（Go 有但 TS 无）:');
    missingInTs.forEach(k => console.log(`   - ${k} (Go: ${JSON.stringify(goConfig[k])})`));
}

if (missingInGo.length > 0) {
    console.log('\n[WARN] Go DefaultConfig 缺失字段（TS 有但 Go 无）:');
    missingInGo.forEach(k => console.log(`   - ${k} (TS: ${JSON.stringify(tsConfig[k])})`));
}

if (mismatches.length > 0) {
    console.log('\n[FAIL] 默认值不一致字段:');
    mismatches.forEach(m => {
        console.log(`   - ${m.key}: Go=${JSON.stringify(m.goVal)} vs TS=${JSON.stringify(m.tsVal)}`);
    });
}

console.log('='.repeat(60));
if (mismatches.length === 0 && missingInTs.length === 0 && missingInGo.length === 0) {
    console.log('[PASS] 所有字段默认值一致！');
    process.exit(0);
} else {
    console.log('[FAIL] 契约比对失败，请同步修改 TS DEFAULT_CONFIG 或 Go DefaultConfig()');
    process.exit(1);
}
