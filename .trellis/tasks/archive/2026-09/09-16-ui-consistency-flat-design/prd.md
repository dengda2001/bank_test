# 全站 UI 一致性审计与 Flat Design 重构

## 目标

在房东收租工作台的全部业务流程交付后，对应用的每一个前端页面进行完整的视觉、交互与响应式检查。先消除现有样式一致性和组件可用性问题，并将这一轮修正作为独立提交；随后以本 PRD 保留的 Flat Design 提示词为设计输入重构 UI；最后完成手机端适配和逐页复查，交付一致、可维护且无已知回归的 UI。

## 前置条件

本任务是父任务 `09-15-landlord-rent-operations` 的最后一个子任务。不得在下列七项均已完成、验证并归档前启动，也不得把它与任一业务子任务的实现提交混在一起：

1. `09-15-rent-ledger-foundation`
2. `09-15-tenant-profile-and-history`
3. `09-15-bank-receipt-management`
4. `09-15-cash-rent-receipts`
5. `09-15-monthly-rent-dashboard`
6. `09-15-monthly-rent-dunning`
7. `09-16-api-e2e-acceptance-cleanup`（必须已完成真实 API 全链路验收并确认测试数据零残留）

启动时还须以最终可运行的路由清单为准，覆盖已有页面及上述流程新增、修改的全部页面、抽屉、弹窗、表格、表单与空／加载／错误状态；不能仅抽查几个主页面。

## 范围与约束

- 第一阶段逐页修复 UI 质量问题：任何前端组件均须可见、可用且可交互；不得存在文字大小或行高不统一、按钮规格或视觉状态不一致、按钮位置错位、组件元素间距失衡、表格／表单文字与控件间距不一致等问题。
- 检查范围包括桌面与项目支持的窄屏断点、键盘焦点、禁用／加载／错误／空数据状态，以及鼠标和键盘可达的交互；修复不能破坏已交付的收租业务流程或数据语义。
- 统一复用或调整已有设计 token、全局样式和组件体系，避免页面级临时样式造成新的漂移。此前定制的组件也必须采用相同的视觉语言和交互规则，不能被遗忘或替换为不可用的实现。
- 第一阶段的样式与可用性修正完成、经全量页面复查通过后，**必须单独提交一次**；该提交不得包含后续 Flat Design 重构内容。
- 仅在上述独立提交存在后，才可进入第二阶段 UI 重构。重构后的功能、路由、表格、表单、筛选、抽屉、弹窗和自定义组件必须保持可用，且不存在已知 UI 或交互回归。
- 在第二阶段 UI 重构完成后，必须进行第三阶段的手机端适配，不能以“桌面端缩窄后勉强可看”替代真实适配。每个页面及其抽屉、弹窗、表格、表单和全部状态均要在常见手机宽度（最小 320 CSS px，以及 360、375、390 和 412 CSS px）下验证；触摸目标、信息层级和关键操作必须保持可用。
- 手机端不得出现横向溢出、元素脱离容器或彼此重叠、按钮或文本被裁切、表单标签／输入框／校验信息错位、表格不可读且无替代浏览方式、弹窗／抽屉超出视窗，或因断点切换造成控件位置跳动、状态丢失和交互失效。必要时应使用适合小屏的布局、表格呈现或操作入口，而不是压缩桌面布局。

## 验收标准

- [ ] 七个前置子任务全部完成并归档；本任务的路由／页面状态清单覆盖应用中每个实际可访问页面和相关状态。
- [ ] 每个页面都完成视觉与交互复查：所有组件可渲染、可操作且不会遮挡、溢出或失去焦点；标题、正文、标签、表格和表单的字号、字重、行高与间距遵循统一规则。
- [ ] 同类按钮在尺寸、对齐、位置、圆角、颜色、禁用／悬停／焦点状态和反馈上保持一致；表格、表单、弹窗、抽屉和自定义组件的内外边距一致且响应式可用。
- [ ] 第一阶段修复有一个独立提交，并在提交后再次完成全量页面检查；该提交没有混入 Flat Design 重构。
- [ ] 第二阶段遵循下方设计提示词，形成可维护的统一设计系统；所有既有自定义组件在新设计下保持一致、可访问、响应式且无已知功能或视觉回归。
- [ ] 最终以真实交互逐页验证关键流程，确认按钮、表单提交、筛选、表格操作、弹窗／抽屉开关及错误反馈均可用。
- [ ] 第二阶段完成后，每个页面和相关状态均在 320、360、375、390、412 CSS px 的手机视窗完成复查；无横向滚动、元素乱跑／重叠、文字或按钮裁切、表单错位、表格不可用、弹窗／抽屉越界或断点导致的交互失效。
- [ ] 手机端真实触摸操作可完成关键流程：导航、主要按钮、表单填写与校验、筛选、表格查看／操作，以及弹窗／抽屉的打开、关闭和提交；自定义组件与其他组件保持同一响应式规则。

## 第二阶段执行用 UI 生成提示词

仅在第一阶段独立提交和复查通过后使用。以下文本按用户提供内容原样保留，作为第二阶段的设计输入：

<role>
You are an expert frontend engineer, UI/UX designer, visual design specialist, and typography expert. Your goal is to help the user integrate a design system into an existing codebase in a way that is visually consistent, maintainable, and idiomatic to their tech stack.

Before proposing or writing any code, first build a clear mental model of the current system:
- Identify the tech stack (e.g. React, Next.js, Vue, Tailwind, shadcn/ui, etc.).
- Understand the existing design tokens (colors, spacing, typography, radii, shadows), global styles, and utility patterns.
- Review the current component architecture (atoms/molecules/organisms, layout primitives, etc.) and naming conventions.
- Note any constraints (legacy CSS, design library in use, performance or bundle-size considerations).

Ask the user focused questions to understand the user's goals. Do they want:
- a specific component or page redesigned in the new style,
- existing components refactored to the new system, or
- new pages/features built entirely in the new style?

Once you understand the context and scope, do the following:
- Propose a concise implementation plan that follows best practices, prioritizing:
  - centralizing design tokens,
  - reusability and composability of components,
  - minimizing duplication and one-off styles,
  - long-term maintainability and clear naming.
- When writing code, match the user’s existing patterns (folder structure, naming, styling approach, and component patterns).
- Explain your reasoning briefly as you go, so the user understands *why* you’re making certain architectural or design choices.

Always aim to:
- Preserve or improve accessibility.
- Maintain visual consistency with the provided design system.
- Leave the codebase in a cleaner, more coherent state than you found it.
- Ensure layouts are responsive and usable across devices.
- Make deliberate, creative design choices (layout, motion, interaction details, and typography) that express the design system’s personality instead of producing a generic or boilerplate UI.

</role>

<design-system>
# Design Philosophy
**Flat Design** removes all artifice. It rejects the illusion of three-dimensionality—no drop shadows, no bevels, no realistic gradients, no textures. It relies entirely on **hierarchy through size, color, and typography**. This is not minimalism for the sake of being minimal; it's **confident reduction** that creates visual interest through pure form.

The aesthetic is **digital-native but print-inspired**: crisp edges, solid blocks of color, and a strict reliance on the grid. It communicates clarity, efficiency, and modernity. It is not "boring" or "plain"; it is **boldly reductive and graphic**. Every element exists because it is necessary. Visual interest comes from the strategic interplay of solid shapes, vibrant (but controlled) color palettes, and dynamic scale.

**Core Principles:**
1.  **Zero Artificial Depth**: The Z-axis does not exist. Everything is on the same plane. However, visual hierarchy is created through scale, color contrast, and strategic layering of flat shapes.
2.  **Color as Structure**: Bold background colors define sections and grouping, not lines or shadows. Color transitions are sharp, never blurred or gradual.
3.  **Typography as Interface**: Text size and weight bear the load of hierarchy. Typography is geometric, bold, and demands attention.
4.  **Geometric Purity**: Rectangles, circles, and squares dominate. Rounded corners are consistent and moderate. No organic blobs or complex shapes.
5.  **Interactive Feedback**: Hover states are pronounced through color shifts, scale transformations, and instant transitions—never through shadow depth.
6.  **Strategic Decoration**: Large, subtle geometric shapes in background create visual interest without breaking the flat aesthetic—think poster design.

# Design Token System

## Colors (Single Palette: Light Mode)
A vibrant, confident palette that avoids muddy tones. High contrast is essential.

-   **Background**: `#FFFFFF` (Pure White) - The canvas.
-   **Foreground**: `#111827` (Gray 900) - Sharp, high-contrast text.
-   **Primary**: `#3B82F6` (Blue 500) - The "Action" color. Bright, standard digital blue.
-   **Secondary**: `#10B981` (Emerald 500) - Supporting accent.
-   **Accent**: `#F59E0B` (Amber 500) - For highlights/badges.
-   **Muted**: `#F3F4F6` (Gray 100) - Used for secondary backgrounds/blocks.
-   **Border**: `#E5E7EB` (Gray 200) - Used sparingly.

## Typography
**Font Family**: **'Outfit', sans-serif**.
A geometric sans-serif that mirrors the shapes of the UI.
-   **Headings**: Bold (700) or Extra Bold (800). Tight letter-spacing (`-0.02em`).
-   **Body**: Regular (400). Readable, standard spacing.
-   **Labels/Buttons**: Medium (500) or SemiBold (600). Uppercase often used for labels (`tracking-wider`).

## Radius & Shapes
-   **Radius**: `rounded-md` (6px) or `rounded-lg` (8px). Consistent throughout. Not fully rounded (pill) unless it's a tag.
-   **Borders**: generally `0px`. We use background colors to define edges. If a border is needed (e.g., inputs), `border-2` solid color.

## Shadows & Effects
-   **Shadows**: `shadow-none`. **ABSOLUTELY NO BOX SHADOWS ON ELEMENTS.**
-   **Gradients**: Only subtle directional gradients for background decoration (e.g., `from-[#F3F4F6] to-transparent`). Never on buttons or cards. Never colorful or vibrant gradients.
-   **Blur**: None on elements. No backdrop-blur effects.
-   **Background Decoration**: Large geometric shapes with low opacity (`bg-white/5`) positioned absolutely for visual interest.

# Component Stylings

## Buttons
-   **Primary**: Solid Primary color background. White text. `rounded-md`. Height `h-14` to `h-16` for good touch targets. `transition-all duration-200 hover:scale-105` (scale transformation for feedback). Color shift on hover (e.g., `hover:bg-blue-600`). No shadow.
-   **Secondary**: Solid Muted background (Gray 100). Dark text. `hover:bg-gray-200` with scale effect.
-   **Outline**: `border-4` solid color (not border-2 for more boldness). Text matches border color. Transparent bg. `hover:bg-[color] hover:text-white` (fill effect on hover).

## Cards
-   **Style**: "Color Block".
-   **Appearance**: Solid background color (White on Gray page, or soft color tints like `bg-blue-50`, `bg-green-50` for features). No shadow. No border. Padding is generous (`p-6` or `p-8`). Rounded corners `rounded-lg`.
-   **Interaction**: `group cursor-pointer transition-all duration-200 hover:scale-[1.02]` (subtle scale). For colored backgrounds, add `hover:bg-[color]-100` for intensification. Icons within cards can have `group-hover:scale-110`.

## Inputs
-   **Normal**: Gray 100 background (`bg-gray-100`). No border. Text Gray 900. `rounded-md`.
-   **Focus**: White background. `border-2` solid Primary. No focus ring glow, just the hard border.

## Section Stylings
-   **Alternating Backgrounds**: Use White vs. Gray 100 (`#F3F4F6`) vs. Bold accent colors (Primary Blue, Emerald, Amber) to distinguish page sections. Sharp color transitions between sections.
-   **Dividers**: No thin line dividers between sections. Use whitespace or color blocks. Exception: FAQ uses thick `border-2` between items for structure.
-   **Background Decoration**: Use `absolute` positioned geometric shapes with low opacity or subtle gradients for visual interest. Examples: large circles (`rounded-full`), rotated squares, gradient overlays (`from-[color] to-transparent`).

# Iconography
-   **Library**: `lucide-react`.
-   **Style**: Standard to bold stroke (2px to 2.5px for emphasis).
-   **Treatment**: Often placed inside a solid colored circle (white circle with colored icon like `bg-white text-blue-600`). Circle size `h-14 w-14` or `h-16 w-16`.
-   **Animation**: `transition-transform duration-200 group-hover:scale-110` for icons within cards. Simple color intensity shifts on hover.

# Layout & Spacing
-   **Container**: `max-w-7xl`.
-   **Grid**: Rigid. 12-column base. Elements align perfectly.
-   **Spacing**: Comfortable but structured. Multiples of 4 (Tailwind default).
-   **Density**: Medium. Not too airy, not too dense. "Functional".

# Motion
-   **Vibe**: "Digital", "Snappy", "Direct".
-   **Transitions**: `transition-all duration-200` for most interactions. `duration-300` for larger transformations.
-   **Hover**: Immediate visual feedback through:
    - Scale transformations (`hover:scale-105` for buttons, `hover:scale-[1.02]` for cards)
    - Color shifts (darkening or lightening)
    - Color fills (outline buttons filling with color)
    - Icon scaling within cards (`group-hover:scale-110`)

# Accessibility
-   **Focus Rings**: Since we have no shadows, focus states must use high-contrast `ring-2 ring-offset-2 ring-blue-500` or similar solid outlines.
-   **Contrast**: Text on colored backgrounds must pass WCAG AA (e.g., White text on Blue 500 is okay, but check carefully with lighter accents).

# Non-Genericness / "The Bold Factor"
-   **Avoid**: "Material Design" floating cards, generic Bootstrap layouts, subtle pastels everywhere.
-   **Emphasize**: The "Poster" look. Treat every section like a flat graphic poster with bold color blocking.
-   **Bold Choices Implemented**:
    - **Large decorative geometric shapes** in hero background (circles, rotated squares with low opacity)
    - **Vibrant full-section color blocks** (Blue hero, Emerald benefits, Amber CTA, Dark gray How It Works & Footer)
    - **Dramatic scale effects** on pricing cards (popular tier starts larger and scales more)
    - **Multi-color stat numbers** (each stat uses a different accent color)
    - **Abstract geometric compositions** (overlapping shapes in hero illustration and benefits section)
    - **Pronounced hover states** (scale, color intensification, fills)
    - **Bold typography** with tight leading and strong weight contrast
    - **Thick borders** (border-4 on outline buttons, border-2 on FAQ items)
-   **Visual Interest Without Depth**: Achieved through color contrast, geometric layering, and scale—never shadows or gradients.
</design-system>
