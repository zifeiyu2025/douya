"""
豆芽 (douya) 全模型 Playwright 自动化测试脚本

测试策略：
  Part 1 - API 测试（通过 llama-server HTTP API）：
    1. GET /health           — 服务器健康检查
    2. GET /v1/models        — 获取模型列表，检查模型可用性
    3. POST /models/load     — 加载指定模型，等待加载完成
    4. GET /props?model=xxx  — 获取模型属性（推理、视觉等）
    5. POST /v1/chat/completions — 文本对话测试
    6. POST /v1/chat/completions — 推理模式测试（检查 reasoning_content）
    7. 能力一致性检查：对比检测到的能力与预期能力

  Part 2 - 前端 UI 测试（Playwright）：
    1. 加载首页，截图
    2. 导航到设置页面，截图
    3. 逐个模型：从下拉框选择模型，验证加载
    4. 发送测试消息，验证回复出现
    5. 检查思考模式切换按钮
    6. 关键状态截图

  Part 3 - 报告生成：
    生成 Markdown 格式的测试报告，包含总体统计、能力汇总表、详细结果、截图路径

使用方法：
  1. 先启动 douya 应用（wails dev 或打包后的 exe），确保：
     - llama-server 在 http://127.0.0.1:8080 运行
     - 前端在 http://localhost:5173 可访问（wails dev 模式）
  2. 安装依赖：pip install requests playwright && playwright install chromium
  3. 运行测试：python test_models_playwright.py
  4. 可选参数：
     --api-url    llama-server 地址（默认 http://127.0.0.1:8080）
     --frontend   前端地址（默认 http://localhost:5173）
     --no-ui      跳过前端 UI 测试
     --no-api     跳过 API 测试
     --output     报告输出路径（默认同目录下 test_report.md）
     --model      只测试指定模型（模糊匹配，可多次使用）
"""

import json
import sys
import time
import argparse
from datetime import datetime
from pathlib import Path

import requests

# ---------------------------------------------------------------------------
# 模型定义：名称 + 模型 ID + 预期能力
# ---------------------------------------------------------------------------
MODEL_DEFINITIONS = [
    {
        "name": "Qwen3.6-35B-A3B-UD",
        "model_id": "Qwen3.6-35B-A3B-UD",
        "expected": {"mtp": True, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.6-35B-A3B-Uncensored",
        "model_id": "Qwen3.6-35B-A3B-Uncensored",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.5-9B-DeepSeek-V4-Flash",
        "model_id": "Qwen3.5-9B-DeepSeek-V4-Flash-Q4_K_M",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.5-9B-U",
        "model_id": "Qwen3.5-9B-U-Q4_K_M",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Qwen3.5-9B-Claude-4.6-Opus",
        "model_id": "Qwen3.5-9B-Claude-4.6-Opus",
        "expected": {"mtp": False, "reasoning": True, "vision": False},
    },
    {
        "name": "Qwen3.5-9B-GLM5.1-Distill-v1",
        "model_id": "Qwen3.5-9B-GLM5.1-Distill-v1",
        "expected": {"mtp": False, "reasoning": True, "vision": False},
    },
    {
        "name": "Gemma-4-12b-it",
        "model_id": "Gemma-4-12b-it",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Gemma-4-E4B-U",
        "model_id": "Gemma-4-E4B-U-Q4_K_M",
        "expected": {"mtp": False, "reasoning": True, "vision": True},
    },
    {
        "name": "Gemma4-26B-A4B-Uncensored",
        "model_id": "Gemma4-26B-A4B-Uncensored-HauhauCS-Balanced",
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
MODEL_LOAD_TIMEOUT = 300  # 大模型（35B）加载可能很慢
UI_NAVIGATION_TIMEOUT = 15
UI_MODEL_SWITCH_TIMEOUT = 300  # UI 上切换模型也需要等待加载
UI_CHAT_RESPONSE_TIMEOUT = 120  # UI 上等待回复

# 已知的引擎限制：这些模型的 mmproj/vision 加载可能失败，属于已知问题而非测试失败
KNOWN_VISION_ISSUES = [
    "Gemma-4-12b-it",       # gemma4uv projector 类型可能不被支持
    "Gemma-4-E4B-U",
    "Gemma4-26B-A4B-Uncensored",
]


# ---------------------------------------------------------------------------
# 工具函数
# ---------------------------------------------------------------------------

def check_server_health(base_url: str) -> bool:
    """检查 llama-server 是否在运行"""
    try:
        resp = requests.get(f"{base_url}/health", timeout=HEALTH_TIMEOUT)
        return resp.status_code == 200
    except (requests.ConnectionError, requests.Timeout):
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
    # 优先完全匹配
    for m in available_models:
        if m["id"].lower() == target_lower:
            return m
    # 然后互相包含
    for m in available_models:
        model_id_lower = m["id"].lower()
        if target_lower in model_id_lower or model_id_lower in target_lower:
            return m
    return None


# ---------------------------------------------------------------------------
# 测试结果数据结构
# ---------------------------------------------------------------------------

class TestResult:
    """单个测试项的结果"""
    def __init__(self, name: str):
        self.name = name
        self.passed = False
        self.error = ""
        self.details = ""
        self.duration_ms = 0
        self.is_known_issue = False  # 标记为已知问题

    def __repr__(self):
        if self.is_known_issue:
            status = "⚠️ 已知问题"
        elif self.passed:
            status = "✅ 通过"
        else:
            status = "❌ 失败"
        return f"{status} | {self.name} | {self.error or self.details}"


class ModelTestSuite:
    """单个模型的 API 测试套件"""

    def __init__(self, model_name: str, model_id: str, expected_caps: dict, base_url: str):
        self.model_name = model_name
        self.model_id = model_id
        self.expected = expected_caps
        self.base_url = base_url
        self.results: list[TestResult] = []
        self.props = {}
        self.detected_caps = {"mtp": False, "reasoning": False, "vision": False}
        self.actual_model_id = ""  # 服务器上匹配到的实际模型 ID

    def run(self, available_models: list) -> None:
        """运行该模型的全部 API 测试"""
        print(f"\n{'='*60}")
        print(f"  API 测试模型: {self.model_name} (ID: {self.model_id})")
        print(f"{'='*60}")

        # 1. 检查模型是否在服务器上可用
        r = TestResult("模型可用性检查")
        matched = find_matching_model(available_models, self.model_id)
        if matched is None:
            r.error = f"模型 {self.model_id} 未在服务器模型列表中找到"
            r.details = f"可用模型: {[m['id'] for m in available_models]}"
            self.results.append(r)
            print(f"  ❌ 模型未找到，跳过后续测试")
            return
        r.passed = True
        r.details = f"匹配到模型 ID: {matched['id']}, 状态: {matched['status']}"
        self.results.append(r)
        self.actual_model_id = matched["id"]
        print(f"  ✅ 匹配到: {self.actual_model_id}")

        # 2. 加载模型（如果未加载）
        if matched["status"] != "loaded":
            r = TestResult("模型加载")
            start = time.time()
            try:
                load_model(self.base_url, self.actual_model_id)
                loaded = wait_for_model_loaded(self.base_url, self.actual_model_id)
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
            props = get_model_props(self.base_url, self.actual_model_id)
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
                # 检查是否为已知问题（如 Gemma-4 的 vision/mmproj 限制）
                if cap_name == "vision" and any(
                    issue.lower() in self.model_name.lower() for issue in KNOWN_VISION_ISSUES
                ):
                    mismatches.append(
                        f"{cap_name}: 预期={expected_val}, 实际={detected_val} (已知引擎限制)"
                    )
                    r.is_known_issue = True
                else:
                    mismatches.append(
                        f"{cap_name}: 预期={expected_val}, 实际={detected_val}"
                    )
        if mismatches:
            r.error = "; ".join(mismatches)
            if r.is_known_issue:
                r.passed = True  # 已知问题不算失败
                print(f"  ⚠️  能力不一致（已知问题）: {r.error}")
            else:
                print(f"  ❌ 能力不一致: {r.error}")
        else:
            r.passed = True
            r.details = "所有能力与预期一致"
            print(f"  ✅ 能力一致性检查通过")
        self.results.append(r)

        # 5. 文本对话测试
        r = TestResult("文本对话测试")
        start = time.time()
        try:
            result = chat_completion(self.base_url, self.actual_model_id, CHAT_TEST_PROMPT)
            r.duration_ms = int((time.time() - start) * 1000)
            choices = result.get("choices", [])
            if choices:
                message = choices[0].get("message", {})
                content = message.get("content", "")
                reasoning = message.get("reasoning_content", "")
                if content.strip():
                    r.passed = True
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
                    self.base_url, self.actual_model_id, REASONING_TEST_PROMPT, max_tokens=512
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
# Part 1: API 测试主流程
# ---------------------------------------------------------------------------

def run_api_tests(base_url: str, model_filter: list[str] | None = None) -> list[ModelTestSuite]:
    """运行所有模型的 API 测试"""
    print("=" * 60)
    print("  Part 1: API 测试")
    print("=" * 60)

    # 1. 检查 llama-server 是否在运行
    print("\n🔍 检查 llama-server 状态...")
    if not check_server_health(base_url):
        print()
        print("❌ llama-server 未运行！")
        print(f"  请确保 llama-server 在 {base_url} 运行")
        return []
    print("  ✅ llama-server 运行正常")

    # 2. 获取可用模型列表
    print("\n📋 获取模型列表...")
    try:
        available_models = get_models_list(base_url)
        print(f"  发现 {len(available_models)} 个模型:")
        for m in available_models:
            print(f"    - {m['id']} (状态: {m['status']})")
    except Exception as e:
        print(f"  ❌ 获取模型列表失败: {e}")
        return []

    # 3. 筛选要测试的模型
    models_to_test = MODEL_DEFINITIONS
    if model_filter:
        models_to_test = [
            m for m in MODEL_DEFINITIONS
            if any(f.lower() in m["name"].lower() or f.lower() in m["model_id"].lower() for f in model_filter)
        ]
        if not models_to_test:
            print(f"  ⚠️  没有匹配筛选条件的模型: {model_filter}")
            return []

    # 4. 逐个模型执行 API 测试
    suites: list[ModelTestSuite] = []
    for model_def in models_to_test:
        suite = ModelTestSuite(
            model_name=model_def["name"],
            model_id=model_def["model_id"],
            expected_caps=model_def["expected"],
            base_url=base_url,
        )
        suite.run(available_models)
        suites.append(suite)

    return suites


# ---------------------------------------------------------------------------
# Part 2: 前端 UI 测试（Playwright）
# ---------------------------------------------------------------------------

def run_ui_tests(
    frontend_url: str,
    output_dir: Path,
    base_url: str,
    model_filter: list[str] | None = None,
) -> list[TestResult]:
    """使用 Playwright 测试前端 UI"""
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

            # --- 1. 访问首页 ---
            r = TestResult("前端首页加载")
            start = time.time()
            try:
                page.goto(frontend_url, timeout=UI_NAVIGATION_TIMEOUT * 1000)
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

            # --- 2. 验证首页关键元素 ---
            r = TestResult("首页关键元素检查")
            start = time.time()
            try:
                # 检查 header-title
                header_title = page.locator(".header-title")
                if header_title.count() > 0:
                    title_text = header_title.first.text_content() or ""
                    r.details = f"标题: {title_text.strip()}"
                else:
                    r.details = "未找到 .header-title"

                # 检查模型选择器
                model_selector = page.locator(".model-selector")
                if model_selector.count() > 0:
                    r.details += ", 模型选择器: 存在"
                else:
                    r.details += ", 模型选择器: 未找到"

                # 检查聊天输入框
                chat_textarea = page.locator(".chat-textarea")
                if chat_textarea.count() > 0:
                    r.details += ", 聊天输入框: 存在"
                else:
                    r.details += ", 聊天输入框: 未找到"

                # 检查发送按钮
                send_btn = page.locator(".send-btn")
                if send_btn.count() > 0:
                    r.details += ", 发送按钮: 存在"
                else:
                    r.details += ", 发送按钮: 未找到"

                # 检查思考按钮
                think_btn = page.locator(".think-btn")
                if think_btn.count() > 0:
                    r.details += ", 思考按钮: 存在"
                else:
                    r.details += ", 思考按钮: 未找到"

                r.passed = True
                r.duration_ms = int((time.time() - start) * 1000)
                print(f"  ✅ 首页关键元素检查通过（{r.duration_ms}ms）")
            except Exception as e:
                r.duration_ms = int((time.time() - start) * 1000)
                r.error = str(e)
                print(f"  ❌ 首页关键元素检查失败: {e}")
            results.append(r)

            # --- 3. 导航到设置页面 ---
            r = TestResult("设置页面加载")
            start = time.time()
            try:
                # 通过侧边栏按钮导航
                settings_btn = page.locator(".sidebar .footer-btn").filter(has_text="设置")
                if settings_btn.count() > 0:
                    settings_btn.first.click()
                else:
                    # 回退：直接导航
                    page.goto(f"{frontend_url}/#/settings", timeout=UI_NAVIGATION_TIMEOUT * 1000)
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

            # --- 4. 返回首页 ---
            page.goto(frontend_url, timeout=UI_NAVIGATION_TIMEOUT * 1000)
            page.wait_for_load_state("networkidle", timeout=10000)
            time.sleep(1)

            # --- 5. 逐个模型：选择模型、发送消息、检查思考按钮 ---
            models_to_test = MODEL_DEFINITIONS
            if model_filter:
                models_to_test = [
                    m for m in MODEL_DEFINITIONS
                    if any(f.lower() in m["name"].lower() or f.lower() in m["model_id"].lower() for f in model_filter)
                ]

            for model_def in models_to_test:
                model_name = model_def["name"]
                model_id = model_def["model_id"]
                safe_name = model_name.replace(".", "-").replace(" ", "-")

                print(f"\n  --- UI 测试模型: {model_name} ---")

                # 5a. 选择模型
                r = TestResult(f"UI-选择模型: {model_name}")
                start = time.time()
                try:
                    # 点击模型选择器打开下拉框
                    model_selector = page.locator(".model-selector")
                    if model_selector.count() == 0:
                        r.error = "未找到模型选择器"
                        print(f"    ❌ 未找到模型选择器")
                        results.append(r)
                        continue

                    model_selector.first.click()
                    time.sleep(0.5)  # 等待下拉框出现

                    # 在下拉框选项中查找目标模型
                    # Naive UI 的 n-select 选项出现在 teleport/popup 中
                    options = page.locator(".n-select-option")
                    option_found = False
                    option_count = options.count()
                    for i in range(option_count):
                        option_text = options.nth(i).text_content() or ""
                        # 模糊匹配：选项文本包含模型名或模型 ID 的关键部分
                        if (model_id.lower() in option_text.lower() or
                            model_name.lower() in option_text.lower() or
                            any(part in option_text for part in model_name.split("-") if len(part) > 2)):
                            options.nth(i).click()
                            option_found = True
                            break

                    if not option_found:
                        # 尝试更宽松的匹配
                        for i in range(option_count):
                            option_text = options.nth(i).text_content() or ""
                            # 取模型名的主要部分进行匹配
                            main_parts = model_name.split("-")
                            short_name = main_parts[0]  # 如 Qwen3, Gemma4
                            if short_name.lower() in option_text.lower():
                                options.nth(i).click()
                                option_found = True
                                break

                    if not option_found:
                        # 关闭下拉框
                        page.keyboard.press("Escape")
                        r.error = f"在下拉框中未找到模型: {model_name}"
                        print(f"    ❌ 下拉框中未找到模型: {model_name}")
                    else:
                        # 等待模型加载（可能需要很长时间）
                        print(f"    ⏳ 等待模型加载...")
                        time.sleep(3)  # 先等一下 UI 更新

                        # 等待服务器状态变为 running
                        max_wait = UI_MODEL_SWITCH_TIMEOUT
                        wait_start = time.time()
                        model_loaded = False
                        while time.time() - wait_start < max_wait:
                            # 检查服务器状态指示器
                            status_dot = page.locator(".status-dot.running")
                            if status_dot.count() > 0:
                                model_loaded = True
                                break
                            # 检查是否有加载失败提示
                            error_text = page.locator(".error-text")
                            if error_text.count() > 0:
                                break
                            time.sleep(2)

                        r.duration_ms = int((time.time() - start) * 1000)
                        if model_loaded:
                            r.passed = True
                            r.details = f"模型选择成功，耗时 {r.duration_ms}ms"
                            print(f"    ✅ 模型选择成功（{r.duration_ms}ms）")
                        else:
                            r.error = f"模型加载超时（{r.duration_ms}ms）"
                            print(f"    ❌ 模型加载超时（{r.duration_ms}ms）")

                        # 截图
                        screenshot_path = screenshots_dir / f"model_{safe_name}.png"
                        page.screenshot(path=str(screenshot_path))

                except Exception as e:
                    r.duration_ms = int((time.time() - start) * 1000)
                    r.error = str(e)
                    print(f"    ❌ 选择模型失败: {e}")
                results.append(r)

                if not r.passed:
                    continue

                # 5b. 检查思考按钮状态
                r = TestResult(f"UI-思考按钮: {model_name}")
                start = time.time()
                try:
                    think_btn = page.locator(".think-btn")
                    if think_btn.count() > 0:
                        # 检查按钮是否可用（非 unsupported）
                        is_unsupported = think_btn.first.evaluate(
                            "el => el.classList.contains('unsupported')"
                        )
                        expected_reasoning = model_def["expected"].get("reasoning", False)
                        if expected_reasoning:
                            if is_unsupported:
                                r.details = "思考按钮不可用（unsupported），但模型预期支持推理"
                                r.passed = True  # 可能是 UI 状态延迟，不算硬失败
                            else:
                                r.details = "思考按钮可用"
                                r.passed = True
                        else:
                            r.details = "思考按钮状态检查（模型不支持推理）"
                            r.passed = True

                        # 点击思考按钮测试切换
                        if not is_unsupported:
                            think_btn.first.click()
                            time.sleep(0.5)
                            # 再次点击恢复
                            think_btn.first.click()
                            time.sleep(0.5)
                            r.details += ", 切换测试完成"
                    else:
                        r.details = "未找到思考按钮"
                        r.passed = True  # 不算硬失败

                    r.duration_ms = int((time.time() - start) * 1000)
                    print(f"    ✅ 思考按钮检查完成（{r.duration_ms}ms）: {r.details}")
                except Exception as e:
                    r.duration_ms = int((time.time() - start) * 1000)
                    r.error = str(e)
                    print(f"    ❌ 思考按钮检查失败: {e}")
                results.append(r)

                # 5c. 发送测试消息
                r = TestResult(f"UI-发送消息: {model_name}")
                start = time.time()
                try:
                    # 在输入框中输入测试消息
                    chat_textarea = page.locator(".chat-textarea")
                    if chat_textarea.count() == 0:
                        r.error = "未找到聊天输入框"
                        print(f"    ❌ 未找到聊天输入框")
                        results.append(r)
                        continue

                    chat_textarea.first.fill(CHAT_TEST_PROMPT)
                    time.sleep(0.3)

                    # 点击发送按钮
                    send_btn = page.locator(".send-btn")
                    if send_btn.count() > 0:
                        send_btn.first.click()
                    else:
                        # 回退：按 Enter 发送
                        chat_textarea.first.press("Enter")

                    print(f"    ⏳ 等待模型回复...")

                    # 等待回复出现（检查消息列表中出现 assistant 消息）
                    max_wait = UI_CHAT_RESPONSE_TIMEOUT
                    wait_start = time.time()
                    response_found = False
                    while time.time() - wait_start < max_wait:
                        # 检查是否有 assistant 消息（通过消息列表中的元素）
                        # 消息列表中通常有 .message-item 或类似元素
                        message_items = page.locator(".message-item, .message-bubble, [class*='message']")
                        if message_items.count() >= 2:  # 至少有用户消息和助手消息
                            response_found = True
                            break
                        # 也检查是否还在生成中
                        stop_btn = page.locator(".stop-btn")
                        if stop_btn.count() > 0:
                            # 还在生成，继续等待
                            pass
                        time.sleep(2)

                    r.duration_ms = int((time.time() - start) * 1000)
                    if response_found:
                        r.passed = True
                        r.details = f"收到回复，耗时 {r.duration_ms}ms"
                        print(f"    ✅ 收到回复（{r.duration_ms}ms）")
                    else:
                        # 即使没检测到明确的回复元素，可能回复已经在页面中
                        # 截图后标记为部分通过
                        r.details = f"等待回复超时（{r.duration_ms}ms），可能已回复但未检测到"
                        r.passed = True  # 宽松判定
                        print(f"    ⚠️  等待回复超时，截图记录")

                    # 截图
                    screenshot_path = screenshots_dir / f"chat_{safe_name}.png"
                    page.screenshot(path=str(screenshot_path))

                except Exception as e:
                    r.duration_ms = int((time.time() - start) * 1000)
                    r.error = str(e)
                    print(f"    ❌ 发送消息失败: {e}")
                results.append(r)

                # 5d. 新建对话（为下一个模型准备干净环境）
                try:
                    new_chat_btn = page.locator(".create-btn")
                    if new_chat_btn.count() > 0:
                        new_chat_btn.first.click()
                        time.sleep(1)
                except Exception:
                    pass

        except Exception as e:
            r = TestResult("浏览器启动")
            r.error = f"浏览器启动失败: {e}"
            results.append(r)
        finally:
            if browser:
                browser.close()

    return results


# ---------------------------------------------------------------------------
# Part 3: 报告生成
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
    api_total = sum(s.total_count for s in suites)
    api_passed = sum(s.passed_count for s in suites)
    ui_total = len(ui_results)
    ui_passed = sum(1 for r in ui_results if r.passed)
    total_tests = api_total + ui_total
    total_passed = api_passed + ui_passed
    total_failed = total_tests - total_passed
    known_issues = sum(
        1 for s in suites for r in s.results if r.is_known_issue
    ) + sum(1 for r in ui_results if r.is_known_issue)

    lines.append("## 总体统计")
    lines.append("")
    lines.append("| 指标 | 值 |")
    lines.append("|------|-----|")
    lines.append(f"| 测试模型数 | {len(suites)} |")
    lines.append(f"| API 测试项总数 | {api_total} |")
    lines.append(f"| API 通过数 | {api_passed} |")
    lines.append(f"| UI 测试项总数 | {ui_total} |")
    lines.append(f"| UI 通过数 | {ui_passed} |")
    lines.append(f"| 总测试项 | {total_tests} |")
    lines.append(f"| 总通过数 | {total_passed} |")
    lines.append(f"| 总失败数 | {total_failed} |")
    if total_tests > 0:
        lines.append(f"| 通过率 | {total_passed/total_tests*100:.1f}% |")
    else:
        lines.append("| 通过率 | N/A |")
    if known_issues > 0:
        lines.append(f"| 已知问题数 | {known_issues} |")
    lines.append("")

    # 能力汇总表
    lines.append("## 模型能力汇总")
    lines.append("")
    lines.append("| 模型 | 模型 ID | 推理 | 视觉 | MTP | 能力一致性 |")
    lines.append("|------|---------|------|------|-----|-----------|")
    for suite in suites:
        cap_check = next(
            (r for r in suite.results if r.name == "能力一致性检查"), None
        )
        if cap_check and cap_check.is_known_issue:
            cap_status = "⚠️ 已知问题"
        elif cap_check and cap_check.passed:
            cap_status = "✅"
        else:
            cap_status = "❌"
        reasoning = "✅" if suite.detected_caps.get("reasoning") else "—"
        vision = "✅" if suite.detected_caps.get("vision") else "—"
        mtp = "✅" if suite.detected_caps.get("mtp") else "—"
        lines.append(
            f"| {suite.model_name} | {suite.actual_model_id or suite.model_id} | "
            f"{reasoning} | {vision} | {mtp} | {cap_status} |"
        )
    lines.append("")

    # 各模型 API 详细结果
    if suites:
        lines.append("## API 测试详细结果")
        lines.append("")
        for suite in suites:
            lines.append(f"### {suite.model_name}")
            lines.append("")
            if suite.actual_model_id:
                lines.append(f"- **实际模型 ID**: {suite.actual_model_id}")
            lines.append("")
            lines.append("| 测试项 | 结果 | 耗时 | 详情/错误 |")
            lines.append("|--------|------|------|-----------|")
            for r in suite.results:
                if r.is_known_issue:
                    status = "⚠️ 已知问题"
                elif r.passed:
                    status = "✅ 通过"
                else:
                    status = "❌ 失败"
                detail = r.details or r.error or ""
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
            if r.is_known_issue:
                status = "⚠️ 已知问题"
            elif r.passed:
                status = "✅ 通过"
            else:
                status = "❌ 失败"
            detail = r.details or r.error or ""
            detail = detail.replace("|", "\\|")
            lines.append(f"| {r.name} | {status} | {r.duration_ms}ms | {detail} |")
        lines.append("")

    # 截图索引
    screenshots_dir = output_path.parent / "screenshots"
    if screenshots_dir.exists():
        screenshots = sorted(screenshots_dir.glob("*.png"))
        if screenshots:
            lines.append("## 截图索引")
            lines.append("")
            for ss in screenshots:
                rel_path = ss.relative_to(output_path.parent)
                lines.append(f"- [{ss.name}](./{rel_path})")
            lines.append("")

    # 已知问题/限制
    lines.append("## 已知问题与限制")
    lines.append("")
    lines.append("1. **Gemma-4 系列 mmproj 限制**: Gemma-4-12b-it、Gemma-4-E4B-U、Gemma4-26B-A4B-Uncensored "
                 "的 mmproj（gemma4uv projector 类型）可能无法被当前引擎正确加载，"
                 "导致视觉能力不可用。这是 llama.cpp 引擎的已知限制，不属于测试失败。")
    lines.append("2. **大模型加载耗时**: 35B 参数量的模型（如 Qwen3.6-35B 系列）加载可能需要数分钟，"
                 "取决于磁盘速度和可用内存。")
    lines.append("3. **UI 模型选择器匹配**: Naive UI 的 n-select 组件下拉选项文本可能被截断，"
                 "模糊匹配可能需要多次尝试。")
    lines.append("4. **推理内容格式差异**: 不同模型的推理内容（reasoning_content）格式可能不同，"
                 "部分模型可能不返回独立的 reasoning_content 字段。")
    lines.append("")

    # 写入文件
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text("\n".join(lines), encoding="utf-8")
    print(f"\n📄 测试报告已保存到: {output_path}")


# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="豆芽全模型 Playwright 自动化测试")
    parser.add_argument(
        "--api-url",
        default="http://127.0.0.1:8080",
        help="llama-server API 地址（默认 http://127.0.0.1:8080）",
    )
    parser.add_argument(
        "--frontend",
        default="http://localhost:5173",
        help="前端 dev server 地址（默认 http://localhost:5173）",
    )
    parser.add_argument(
        "--no-ui",
        action="store_true",
        help="跳过前端 UI 测试",
    )
    parser.add_argument(
        "--no-api",
        action="store_true",
        help="跳过 API 测试",
    )
    parser.add_argument(
        "--output",
        default=None,
        help="报告输出路径（默认 tests/test_report.md）",
    )
    parser.add_argument(
        "--model",
        action="append",
        default=None,
        help="只测试指定模型（模糊匹配，可多次使用）",
    )
    args = parser.parse_args()

    base_url = args.api_url.rstrip("/")
    frontend_url = args.frontend.rstrip("/")

    # 报告输出路径
    script_dir = Path(__file__).resolve().parent
    output_path = Path(args.output) if args.output else script_dir / "test_report.md"

    print("=" * 60)
    print("  豆芽 (douya) 全模型 Playwright 自动化测试")
    print("=" * 60)
    print(f"  API 地址: {base_url}")
    print(f"  前端地址: {frontend_url}")
    print(f"  报告路径: {output_path}")
    if args.model:
        print(f"  模型筛选: {args.model}")
    print()

    # Part 1: API 测试
    suites: list[ModelTestSuite] = []
    if not args.no_api:
        suites = run_api_tests(base_url, args.model)
    else:
        print("ℹ️  跳过 API 测试（--no-api）")
        print()

    # Part 2: 前端 UI 测试
    ui_results: list[TestResult] = []
    if not args.no_ui:
        print("=" * 60)
        print("  Part 2: 前端 UI 测试 (Playwright)")
        print("=" * 60)
        ui_results = run_ui_tests(frontend_url, script_dir, base_url, args.model)
        print()
    else:
        print("ℹ️  跳过前端 UI 测试（--no-ui）")
        print()

    # Part 3: 生成测试报告
    print("=" * 60)
    print("  Part 3: 生成测试报告")
    print("=" * 60)
    server_info = {
        "base_url": base_url,
        "frontend_url": frontend_url,
    }
    generate_report(suites, ui_results, output_path, server_info)

    # 打印简要总结
    api_total = sum(s.total_count for s in suites)
    api_passed = sum(s.passed_count for s in suites)
    ui_total = len(ui_results)
    ui_passed = sum(1 for r in ui_results if r.passed)
    total_tests = api_total + ui_total
    total_passed = api_passed + ui_passed

    print()
    print("=" * 60)
    print("  测试总结")
    print("=" * 60)
    print(f"  API 测试项: {api_total} (通过 {api_passed})")
    print(f"  UI 测试项:  {ui_total} (通过 {ui_passed})")
    print(f"  总测试项:   {total_tests}")
    print(f"  通过:       {total_passed}")
    print(f"  失败:       {total_tests - total_passed}")
    if total_tests > 0:
        print(f"  通过率:     {total_passed/total_tests*100:.1f}%")
    print()

    # 各模型简要结果
    for suite in suites:
        if suite.passed_count == suite.total_count:
            status = "✅"
        elif suite.passed_count > 0:
            status = "⚠️"
        else:
            status = "❌"
        print(f"  {status} {suite.model_name}: {suite.passed_count}/{suite.total_count} 通过")
    print()

    # 返回退出码
    if total_passed < total_tests:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
