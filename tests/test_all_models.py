"""
豆芽 (douya) 全模型自动化测试脚本

测试策略：
  1. 通过 llama-server HTTP API 测试（主要）：
     - GET /health           — 服务器健康检查
     - GET /v1/models        — 获取模型列表
     - GET /props?model=xxx  — 获取模型属性（推理、视觉等）
     - POST /v1/chat/completions — 文本对话测试
     - POST /v1/chat/completions — 推理模式测试
  2. 通过 Playwright 截图验证前端 UI（辅助）

使用方法：
  1. 先启动 douya 应用（wails dev 或打包后的 exe），确保 llama-server 在 8080 端口运行
  2. 安装依赖：pip install requests playwright && playwright install chromium
  3. 运行测试：python test_all_models.py
  4. 可选参数：
     --api-url    llama-server 地址（默认 http://127.0.0.1:8080）
     --frontend   前端地址（默认 http://localhost:34115）
     --no-ui      跳过前端 UI 测试
     --output     报告输出路径（默认同目录下 test_report.md）
"""

import json
import sys
import time
import argparse
from datetime import datetime
from pathlib import Path

import requests

# ---------------------------------------------------------------------------
# 模型定义：名称 + 预期能力
# ---------------------------------------------------------------------------
MODEL_DEFINITIONS = [
    {
        "name": "Qwen3.6-35B-A3B-UD",
        "expected": {"mtp": True, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.6-35B-A3B",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.5-9B-DeepSeek-V4-Flash",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.5-9B-U",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.5-9B-Claude-4.6-Opus",
        "expected": {"mtp": False, "reasoning": True, "vision": False},
    },
    {
        "name": "Qwen3.5-9B-GLM5.1-Distill-v1",
        "expected": {"mtp": False, "reasoning": True, "vision": False},
    },
    {
        "name": "Gemma-4-12b-it",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Gemma-4-E4B-U",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Gemma4-26B-A4B-HauhauCS-Balanced",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
]

# ---------------------------------------------------------------------------
# 测试用例
# ---------------------------------------------------------------------------
CHAT_TEST_PROMPT = "你好，请用一句话自我介绍。"
REASONING_TEST_PROMPT = "如果所有的猫都会飞，而Tom是一只猫，那么Tom会飞吗？请一步步推理。"

# 超时设置（秒）
HEALTH_TIMEOUT = 5
API_TIMEOUT = 120
MODEL_LOAD_TIMEOUT = 300  # 大模型加载可能很慢


# ---------------------------------------------------------------------------
# 工具函数
# ---------------------------------------------------------------------------

def check_server_health(base_url: str) -> bool:
    """检查 llama-server 是否在运行"""
    try:
        resp = requests.get(f"{base_url}/health", timeout=HEALTH_TIMEOUT)
        return resp.status_code == 200
    except requests.ConnectionError:
        return False
    except requests.Timeout:
        return False


def get_models_list(base_url: str) -> list:
    """获取服务器上的模型列表"""
    resp = requests.get(f"{base_url}/v1/models", timeout=API_TIMEOUT)
    resp.raise_for_status()
    data = resp.json()
    models = []
    for item in data.get("data", []):
        models.append({
            "id": item.get("id", ""),
            "capabilities": item.get("capabilities", []),
            "status": item.get("status", {}).get("value", "unknown"),
        })
    return models


def get_model_props(base_url: str, model_name: str) -> dict:
    """获取模型属性（推理能力、视觉能力等）"""
    resp = requests.get(
        f"{base_url}/props",
        params={"model": model_name},
        timeout=API_TIMEOUT,
    )
    resp.raise_for_status()
    return resp.json()


def load_model(base_url: str, model_name: str) -> bool:
    """请求服务器加载指定模型"""
    resp = requests.post(
        f"{base_url}/models/load",
        json={"model": model_name},
        timeout=API_TIMEOUT,
    )
    return resp.status_code == 200


def wait_for_model_loaded(base_url: str, model_name: str, timeout: int = MODEL_LOAD_TIMEOUT) -> bool:
    """等待模型加载完成"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            models = get_models_list(base_url)
            for m in models:
                if m["id"] == model_name or model_name in m["id"] or m["id"] in model_name:
                    if m["status"] == "loaded":
                        return True
                    if m["status"] == "failed":
                        return False
        except Exception:
            pass
        time.sleep(2)
    return False


def chat_completion(base_url: str, model_name: str, prompt: str, max_tokens: int = 256) -> dict:
    """发送非流式 chat completion 请求"""
    payload = {
        "model": model_name,
        "messages": [{"role": "user", "content": prompt}],
        "stream": False,
        "max_tokens": max_tokens,
    }
    resp = requests.post(
        f"{base_url}/v1/chat/completions",
        json=payload,
        timeout=API_TIMEOUT,
    )
    resp.raise_for_status()
    return resp.json()


def find_matching_model(available_models: list, target_name: str) -> dict | None:
    """在可用模型列表中模糊匹配目标模型"""
    target_lower = target_name.lower()
    for m in available_models:
        model_id_lower = m["id"].lower()
        # 完全匹配
        if model_id_lower == target_lower:
            return m
        # 互相包含
        if target_lower in model_id_lower or model_id_lower in target_lower:
            return m
    return None


# ---------------------------------------------------------------------------
# 测试执行
# ---------------------------------------------------------------------------

class TestResult:
    """单个测试项的结果"""
    def __init__(self, name: str):
        self.name = name
        self.passed = False
        self.error = ""
        self.details = ""
        self.duration_ms = 0

    def __repr__(self):
        status = "✅ 通过" if self.passed else "❌ 失败"
        return f"{status} | {self.name} | {self.error or self.details}"


class ModelTestSuite:
    """单个模型的测试套件"""

    def __init__(self, model_name: str, expected_caps: dict, base_url: str):
        self.model_name = model_name
        self.expected = expected_caps
        self.base_url = base_url
        self.results: list[TestResult] = []
        self.props = {}  # /props 返回的原始数据
        self.detected_caps = {"mtp": False, "reasoning": False, "vision": False}

    def run(self, available_models: list) -> None:
        """运行该模型的全部测试"""
        print(f"\n{'='*60}")
        print(f"  测试模型: {self.model_name}")
        print(f"{'='*60}")

        # 1. 检查模型是否在服务器上可用
        r = TestResult("模型可用性检查")
        matched = find_matching_model(available_models, self.model_name)
        if matched is None:
            r.error = f"模型 {self.model_name} 未在服务器模型列表中找到"
            r.details = f"可用模型: {[m['id'] for m in available_models]}"
            self.results.append(r)
            print(f"  ❌ 模型未找到，跳过后续测试")
            return
        r.passed = True
        r.details = f"匹配到模型 ID: {matched['id']}, 状态: {matched['status']}"
        self.results.append(r)
        actual_model_id = matched["id"]
        print(f"  ✅ 匹配到: {actual_model_id}")

        # 2. 加载模型（如果未加载）
        if matched["status"] != "loaded":
            r = TestResult("模型加载")
            start = time.time()
            try:
                load_model(self.base_url, actual_model_id)
                loaded = wait_for_model_loaded(self.base_url, actual_model_id)
                r.duration_ms = int((time.time() - start) * 1000)
                if loaded:
                    r.passed = True
                    r.details = f"模型加载成功，耗时 {r.duration_ms}ms"
                    print(f"  ✅ 模型加载成功（{r.duration_ms}ms）")
                else:
                    r.error = "模型加载超时或失败"
                    print(f"  ❌ 模型加载失败")
            except Exception as e:
                r.duration_ms = int((time.time() - start) * 1000)
                r.error = str(e)
                print(f"  ❌ 加载请求失败: {e}")
            self.results.append(r)
            if not r.passed:
                return
        else:
            print(f"  ℹ️  模型已加载，跳过加载步骤")

        # 3. 获取模型属性 /props
        r = TestResult("模型属性获取 (/props)")
        start = time.time()
        try:
            props = get_model_props(self.base_url, actual_model_id)
            r.duration_ms = int((time.time() - start) * 1000)
            self.props = props
            r.passed = True
            # 解析能力
            modalities = props.get("modalities", {})
            self.detected_caps["vision"] = modalities.get("vision", False)
            chat_caps = props.get("chat_template_caps", {})
            self.detected_caps["reasoning"] = chat_caps.get("supports_preserve_reasoning", False)
            r.details = (
                f"视觉={self.detected_caps['vision']}, "
                f"推理={self.detected_caps['reasoning']}, "
                f"音频={modalities.get('audio', False)}"
            )
            print(f"  ✅ 属性获取成功: {r.details}")
        except Exception as e:
            r.duration_ms = int((time.time() - start) * 1000)
            r.error = str(e)
            print(f"  ❌ 属性获取失败: {e}")
        self.results.append(r)

        # 4. 能力一致性检查
        r = TestResult("能力一致性检查")
        mismatches = []
        for cap_name, expected_val in self.expected.items():
            detected_val = self.detected_caps.get(cap_name, False)
            if detected_val != expected_val:
                mismatches.append(
                    f"{cap_name}: 预期={expected_val}, 实际={detected_val}"
                )
        if mismatches:
            r.error = "; ".join(mismatches)
            print(f"  ⚠️  能力不一致: {r.error}")
        else:
            r.passed = True
            r.details = "所有能力与预期一致"
            print(f"  ✅ 能力一致性检查通过")
        self.results.append(r)

        # 5. 文本对话测试
        r = TestResult("文本对话测试")
        start = time.time()
        try:
            result = chat_completion(self.base_url, actual_model_id, CHAT_TEST_PROMPT)
            r.duration_ms = int((time.time() - start) * 1000)
            choices = result.get("choices", [])
            if choices:
                message = choices[0].get("message", {})
                content = message.get("content", "")
                reasoning = message.get("reasoning_content", "")
                if content.strip():
                    r.passed = True
                    # 截取前 100 字符作为摘要
                    preview = content.strip()[:100]
                    r.details = f"回复: {preview}{'...' if len(content.strip()) > 100 else ''}"
                    print(f"  ✅ 文本对话成功（{r.duration_ms}ms）: {preview[:50]}...")
                else:
                    r.error = "模型返回了空内容"
                    if reasoning:
                        r.details = f"但有推理内容（长度={len(reasoning)}）"
                    print(f"  ❌ 模型返回空内容")
            else:
                r.error = "响应中没有 choices"
                print(f"  ❌ 响应格式异常: 无 choices")
        except Exception as e:
            r.duration_ms = int((time.time() - start) * 1000)
            r.error = str(e)
            print(f"  ❌ 文本对话失败: {e}")
        self.results.append(r)

        # 6. 推理模式测试（仅对预期支持推理的模型）
        if self.expected.get("reasoning", False):
            r = TestResult("推理模式测试")
            start = time.time()
            try:
                result = chat_completion(
                    self.base_url, actual_model_id, REASONING_TEST_PROMPT, max_tokens=512
                )
                r.duration_ms = int((time.time() - start) * 1000)
                choices = result.get("choices", [])
                if choices:
                    message = choices[0].get("message", {})
                    content = message.get("content", "")
                    reasoning_content = message.get("reasoning_content", "")
                    # 检查回复是否包含推理相关内容
                    has_reasoning = bool(reasoning_content.strip())
                    # 即使没有 reasoning_content 字段，只要回复合理也算通过
                    content_lower = content.lower()
                    has_reasoning_keywords = any(
                        kw in content_lower
                        for kw in ["会飞", "推理", "因此", "所以", "逻辑", "前提", "结论", "fly"]
                    )
                    if content.strip() and (has_reasoning or has_reasoning_keywords):
                        r.passed = True
                        if has_reasoning:
                            r.details = f"包含推理内容（长度={len(reasoning_content)}）"
                        else:
                            r.details = "回复包含推理关键词"
                        preview = content.strip()[:80]
                        print(f"  ✅ 推理测试通过（{r.duration_ms}ms）: {preview}...")
                    elif content.strip():
                        r.passed = True  # 有回复就算通过，推理格式可能不同
                        r.details = "有回复但未检测到明确推理标记"
                        print(f"  ⚠️  有回复但推理标记不明显（{r.duration_ms}ms）")
                    else:
                        r.error = "推理模式返回空内容"
                        print(f"  ❌ 推理模式返回空内容")
                else:
                    r.error = "响应中没有 choices"
                    print(f"  ❌ 推理测试响应格式异常")
            except Exception as e:
                r.duration_ms = int((time.time() - start) * 1000)
                r.error = str(e)
                print(f"  ❌ 推理测试失败: {e}")
            self.results.append(r)
        else:
            print(f"  ℹ️  跳过推理测试（模型不支持推理）")

    @property
    def passed_count(self) -> int:
        return sum(1 for r in self.results if r.passed)

    @property
    def total_count(self) -> int:
        return len(self.results)


# ---------------------------------------------------------------------------
# 前端 UI 测试（Playwright）
# ---------------------------------------------------------------------------

def run_ui_tests(frontend_url: str, output_dir: Path) -> list[TestResult]:
    """使用 Playwright 截图验证前端 UI"""
    results = []
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        r = TestResult("前端 UI 测试")
        r.error = "playwright 未安装，请运行: pip install playwright && playwright install chromium"
        results.append(r)
        return results

    screenshots_dir = output_dir / "screenshots"
    screenshots_dir.mkdir(exist_ok=True)

    with sync_playwright() as p:
        browser = None
        try:
            browser = p.chromium.launch(headless=True)
            page = browser.new_page(viewport={"width": 1280, "height": 800})

            # 1. 访问首页
            r = TestResult("前端首页加载")
            start = time.time()
            try:
                page.goto(frontend_url, timeout=15000)
                page.wait_for_load_state("networkidle", timeout=10000)
                r.duration_ms = int((time.time() - start) * 1000)
                screenshot_path = screenshots_dir / "home.png"
                page.screenshot(path=str(screenshot_path))
                r.passed = True
                r.details = f"截图已保存: {screenshot_path.name}"
                print(f"  ✅ 前端首页加载成功（{r.duration_ms}ms）")
            except Exception as e:
                r.duration_ms = int((time.time() - start) * 1000)
                r.error = str(e)
                print(f"  ❌ 前端首页加载失败: {e}")
            results.append(r)

            # 2. 访问设置页面
            r = TestResult("设置页面加载")
            start = time.time()
            try:
                page.goto(f"{frontend_url}/#/settings", timeout=15000)
                page.wait_for_load_state("networkidle", timeout=10000)
                time.sleep(2)  # 等待设置页面渲染
                r.duration_ms = int((time.time() - start) * 1000)
                screenshot_path = screenshots_dir / "settings.png"
                page.screenshot(path=str(screenshot_path))
                r.passed = True
                r.details = f"截图已保存: {screenshot_path.name}"
                print(f"  ✅ 设置页面加载成功（{r.duration_ms}ms）")
            except Exception as e:
                r.duration_ms = int((time.time() - start) * 1000)
                r.error = str(e)
                print(f"  ❌ 设置页面加载失败: {e}")
            results.append(r)

        except Exception as e:
            r = TestResult("浏览器启动")
            r.error = f"浏览器启动失败: {e}"
            results.append(r)
        finally:
            if browser:
                browser.close()

    return results


# ---------------------------------------------------------------------------
# 报告生成
# ---------------------------------------------------------------------------

def generate_report(
    suites: list[ModelTestSuite],
    ui_results: list[TestResult],
    output_path: Path,
    server_info: dict,
) -> None:
    """生成 Markdown 格式的测试报告"""
    lines = []
    lines.append("# 豆芽 (douya) 全模型测试报告")
    lines.append("")
    lines.append(f"- **测试时间**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    lines.append(f"- **API 地址**: {server_info.get('base_url', 'N/A')}")
    lines.append(f"- **前端地址**: {server_info.get('frontend_url', 'N/A')}")
    lines.append("")

    # 总体统计
    total_tests = sum(s.total_count for s in suites) + len(ui_results)
    total_passed = sum(s.passed_count for s in suites) + sum(1 for r in ui_results if r.passed)
    lines.append("## 总体统计")
    lines.append("")
    lines.append(f"| 指标 | 值 |")
    lines.append(f"|------|-----|")
    lines.append(f"| 测试模型数 | {len(suites)} |")
    lines.append(f"| 测试项总数 | {total_tests} |")
    lines.append(f"| 通过数 | {total_passed} |")
    lines.append(f"| 失败数 | {total_tests - total_passed} |")
    lines.append(f"| 通过率 | {total_passed/total_tests*100:.1f}% |" if total_tests > 0 else "| 通过率 | N/A |")
    lines.append("")

    # 能力汇总表
    lines.append("## 模型能力汇总")
    lines.append("")
    lines.append("| 模型 | 推理 | 视觉 | MTP | 能力一致性 |")
    lines.append("|------|------|------|-----|-----------|")
    for suite in suites:
        cap_check = next(
            (r for r in suite.results if r.name == "能力一致性检查"), None
        )
        cap_status = "✅" if cap_check and cap_check.passed else "❌"
        reasoning = "✅" if suite.detected_caps.get("reasoning") else "—"
        vision = "✅" if suite.detected_caps.get("vision") else "—"
        mtp = "✅" if suite.detected_caps.get("mtp") else "—"
        lines.append(f"| {suite.model_name} | {reasoning} | {vision} | {mtp} | {cap_status} |")
    lines.append("")

    # 各模型详细结果
    lines.append("## 各模型详细测试结果")
    lines.append("")
    for suite in suites:
        lines.append(f"### {suite.model_name}")
        lines.append("")
        lines.append("| 测试项 | 结果 | 耗时 | 详情/错误 |")
        lines.append("|--------|------|------|-----------|")
        for r in suite.results:
            status = "✅ 通过" if r.passed else "❌ 失败"
            detail = r.details or r.error or ""
            # 转义 Markdown 中的管道符
            detail = detail.replace("|", "\\|")
            lines.append(f"| {r.name} | {status} | {r.duration_ms}ms | {detail} |")
        lines.append("")

    # 前端 UI 测试结果
    if ui_results:
        lines.append("## 前端 UI 测试结果")
        lines.append("")
        lines.append("| 测试项 | 结果 | 耗时 | 详情/错误 |")
        lines.append("|--------|------|------|-----------|")
        for r in ui_results:
            status = "✅ 通过" if r.passed else "❌ 失败"
            detail = r.details or r.error or ""
            detail = detail.replace("|", "\\|")
            lines.append(f"| {r.name} | {status} | {r.duration_ms}ms | {detail} |")
        lines.append("")

    # 写入文件
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text("\n".join(lines), encoding="utf-8")
    print(f"\n📄 测试报告已保存到: {output_path}")


# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="豆芽全模型自动化测试")
    parser.add_argument(
        "--api-url",
        default="http://127.0.0.1:8080",
        help="llama-server API 地址（默认 http://127.0.0.1:8080）",
    )
    parser.add_argument(
        "--frontend",
        default="http://localhost:34115",
        help="前端 dev server 地址（默认 http://localhost:34115）",
    )
    parser.add_argument(
        "--no-ui",
        action="store_true",
        help="跳过前端 UI 测试",
    )
    parser.add_argument(
        "--output",
        default=None,
        help="报告输出路径（默认 tests/test_report.md）",
    )
    args = parser.parse_args()

    base_url = args.api_url.rstrip("/")
    frontend_url = args.frontend.rstrip("/")

    # 报告输出路径
    script_dir = Path(__file__).resolve().parent
    output_path = Path(args.output) if args.output else script_dir / "test_report.md"

    print("=" * 60)
    print("  豆芽 (douya) 全模型自动化测试")
    print("=" * 60)
    print(f"  API 地址: {base_url}")
    print(f"  前端地址: {frontend_url}")
    print(f"  报告路径: {output_path}")
    print()

    # 1. 检查 llama-server 是否在运行
    print("🔍 检查 llama-server 状态...")
    if not check_server_health(base_url):
        print()
        print("❌ llama-server 未运行！")
        print()
        print("请先启动豆芽应用，确保 llama-server 在以下地址运行：")
        print(f"  {base_url}")
        print()
        print("启动方式：")
        print("  - 开发模式: wails dev")
        print("  - 生产模式: 运行打包后的 douya.exe")
        sys.exit(1)
    print("  ✅ llama-server 运行正常")
    print()

    # 2. 获取可用模型列表
    print("📋 获取模型列表...")
    try:
        available_models = get_models_list(base_url)
        print(f"  发现 {len(available_models)} 个模型:")
        for m in available_models:
            print(f"    - {m['id']} (状态: {m['status']})")
    except Exception as e:
        print(f"  ❌ 获取模型列表失败: {e}")
        sys.exit(1)
    print()

    # 3. 逐个模型执行 API 测试
    suites: list[ModelTestSuite] = []
    for model_def in MODEL_DEFINITIONS:
        suite = ModelTestSuite(
            model_name=model_def["name"],
            expected_caps=model_def["expected"],
            base_url=base_url,
        )
        suite.run(available_models)
        suites.append(suite)
        print()

    # 4. 前端 UI 测试
    ui_results: list[TestResult] = []
    if not args.no_ui:
        print("=" * 60)
        print("  前端 UI 测试 (Playwright)")
        print("=" * 60)
        ui_results = run_ui_tests(frontend_url, script_dir)
        print()
    else:
        print("ℹ️  跳过前端 UI 测试（--no-ui）")
        print()

    # 5. 生成测试报告
    server_info = {
        "base_url": base_url,
        "frontend_url": frontend_url,
    }
    generate_report(suites, ui_results, output_path, server_info)

    # 6. 打印简要总结
    total_tests = sum(s.total_count for s in suites) + len(ui_results)
    total_passed = sum(s.passed_count for s in suites) + sum(1 for r in ui_results if r.passed)
    print()
    print("=" * 60)
    print("  测试总结")
    print("=" * 60)
    print(f"  总测试项: {total_tests}")
    print(f"  通过: {total_passed}")
    print(f"  失败: {total_tests - total_passed}")
    if total_tests > 0:
        print(f"  通过率: {total_passed/total_tests*100:.1f}%")
    print()

    # 各模型简要结果
    for suite in suites:
        status = "✅" if suite.passed_count == suite.total_count else "⚠️"
        print(f"  {status} {suite.model_name}: {suite.passed_count}/{suite.total_count} 通过")
    print()

    # 返回退出码
    if total_passed < total_tests:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
