# Design System InovaSala / Luminous

> Documento operacional para uso em modelos de IA, geração de interfaces, criação de páginas, prompts visuais, componentes em Tailwind/CSS e consistência de marca.  
> Fonte convertida: `design-system-inovasala(10).html`.  
> Nome exibido no HTML original: **Luminous**. Como o arquivo enviado está nomeado como `design-system-inovasala`, este Markdown mantém a referência dupla **InovaSala / Luminous**.

---

## 1. Resumo executivo do sistema visual

O design system define uma linguagem visual **dark, premium, tecnológica, luminosa e energética**, construída sobre fundo preto profundo, acentos laranja/âmbar, gradientes quentes, brilho difuso, cards com bordas elétricas, tipografia display leve e interfaces minimalistas.

A estética central é de **produto digital sofisticado**, com sensação de painel SaaS, tecnologia avançada, energia, automação e modernidade. O sistema evita excesso de cor: a base é quase sempre preta ou neutra, e o laranja entra como acento estratégico.

### Palavras-chave para IA

- dark interface
- premium SaaS
- futuristic dashboard
- orange glow
- electric border
- luminous accents
- glassmorphism discreto
- high contrast
- minimal interface
- soft gradients
- technical elegance
- warm energy on black
- component library
- design system showcase

---

## 2. Princípios visuais

### 2.1. Base escura dominante

A interface deve usar fundo preto quase absoluto como base visual:

- Fundo principal: `#050505`
- Fundo de card: `#0A0A0A`
- Superfícies elevadas: `neutral-900`
- Elementos translúcidos: `white/5`, `white/10`, `black/40`

Regra: **não clarear demais a interface**. O contraste e a sofisticação dependem do preto profundo.

### 2.2. Acento laranja como energia

O laranja é o elemento de energia e ação. Deve aparecer em:

- CTAs principais
- indicadores ativos
- bordas luminosas
- ícones de destaque
- gradientes
- efeitos de brilho
- métricas positivas
- microdetalhes de navegação

Cores centrais:

- `orange-400`
- `orange-500`
- `orange-600`
- `amber-500`
- `yellow-200` apenas como ponto de luz em gradientes

Regra: **laranja não deve virar fundo dominante da página**. Ele é acento, não base.

### 2.3. Minimalismo com brilho controlado

O sistema usa brilho, blur e gradientes, mas sempre em baixa opacidade. O resultado desejado é elegante, não chamativo.

Usar:

- sombras laranja suaves
- blur grande em fundos
- bordas translúcidas
- halo laranja em cards especiais
- estrelas/fundos discretos

Evitar:

- neon exagerado
- contornos grossos
- fundos coloridos sólidos
- excesso de animação
- componentes visualmente barulhentos

### 2.4. Hierarquia tipográfica forte

Headings grandes usam **Bricolage Grotesque**, peso leve, tracking negativo e presença visual forte. Texto comum usa fonte sans neutra, preferencialmente **Inter**.

---

## 3. Tokens globais

### 3.1. Fontes

| Uso | Fonte | Classe | Peso | Observação |
|---|---|---|---|---|
| Corpo padrão | Inter | `font-sans` | Regular | Fonte base do body |
| Títulos display | Bricolage Grotesque | `font-bricolage` | Light | Usada em headings grandes e textos de impacto |
| Código/metadados | Monospace | `font-mono` | Regular | Usada em specs técnicas como medidas |

CSS base:

```css
body {
  font-family: 'Inter', sans-serif;
  background-color: #050505;
}

.font-bricolage {
  font-family: 'Bricolage Grotesque', sans-serif !important;
}
```

### 3.2. Cores de fundo

| Token | Valor | Uso |
|---|---:|---|
| `bg-primary` | `#050505` | Fundo principal da página |
| `bg-card` | `#0A0A0A` | Interior de cards escuros |
| `neutral-900` | `#171717` aprox. | Superfícies elevadas |
| `neutral-800/80` | `rgba(38,38,38,0.8)` aprox. | Item ativo em navegação |
| `white/5` | `rgba(255,255,255,0.05)` | Overlay sutil |
| `white/10` | `rgba(255,255,255,0.10)` | Overlay/borda padrão |
| `black/40` | `rgba(0,0,0,0.40)` | Campo de busca escuro/translúcido |

Classes recorrentes:

```txt
bg-[#050505]
bg-[#050505]/80
bg-[#0A0A0A]
bg-neutral-900
bg-neutral-800/80
bg-white/5
bg-white/10
bg-white/[0.02]
bg-black/40
bg-transparent
```

### 3.3. Cores de texto

| Token | Classe | Uso |
|---|---|---|
| Texto primário | `text-white` | Títulos e informações principais |
| Texto secundário | `text-neutral-300` | Labels e textos de apoio importantes |
| Texto terciário | `text-neutral-400` | Parágrafos, descrições, links inativos |
| Texto auxiliar | `text-neutral-500` | Metadados, specs, elementos menos importantes |
| Texto em botão claro | `text-black` | Botão secundário branco |
| Texto em CTA quente | `text-[#2c1306]` | Texto escuro sobre gradiente amarelo/laranja |
| Texto de input | `text-zinc-300` | Campo de busca |
| Placeholder | `placeholder:text-neutral-600` ou `placeholder-zinc-600` | Campos de entrada |

### 3.4. Cores de acento

| Token | Hex aproximado | Uso |
|---|---:|---|
| `orange-400` | `#fb923c` | Luz, ícones, pontos ativos, pulse |
| `orange-500` | `#f97316` | Acento principal, indicadores, bordas, sombras |
| `orange-600` | `#ea580c` | Gradientes, CTAs, botões terciários |
| `amber-500` | `#f59e0b` | Highlight quente em gradiente |
| `yellow-200` | `#fef08a` | Topo luminoso de CTA primário |
| `yellow-300` | `#fde047` | Borda luminosa de card elétrico |

Classes recorrentes:

```txt
text-orange-400
text-orange-500
bg-orange-400
bg-orange-500
bg-orange-500/10
bg-orange-500/20
bg-orange-900/10
bg-orange-950/20
ring-orange-500/20
ring-orange-500/30
selection:bg-orange-500/30
```

### 3.5. Bordas, rings e overlays

| Classe | Uso |
|---|---|
| `border-white/5` | Divisórias e bordas muito sutis |
| `border-white/10` | Borda padrão de cards, inputs e badges |
| `border-white/20` | Borda de elementos em destaque ou hover |
| `ring-white/10` | Contorno sutil de card/input |
| `ring-white/40` | Ring interno de CTA primário |
| `ring-orange-500/20` | Badge de tendência |
| `ring-orange-500/30` | Toggle ativo |

Regra: usar bordas finas, translúcidas e discretas. A interface não usa bordas sólidas pesadas.

### 3.6. Raios de borda

| Classe | Uso |
|---|---|
| `rounded-full` | Botões pill, nav pills, toggles |
| `rounded-md` | Tags pequenas |
| `rounded-lg` | Inputs, badges, blocos pequenos |
| `rounded-xl` | Cards padrão e containers |
| `rounded-[30px]` | Interior de card elétrico |
| `rounded-[32px]` | Container externo de card elétrico |

### 3.7. Sombras e brilhos

| Token/classe | Uso |
|---|---|
| `shadow-[0_0_15px_-3px_rgba(249,115,22,0.4)]` | CTA pequeno no nav |
| `shadow-[0_0_40px_-5px_rgba(249,115,22,0.6)]` | CTA primário grande |
| `hover:shadow-[0_0_60px_-5px_rgba(249,115,22,0.8)]` | Hover de CTA primário |
| `shadow-[0_4px_15px_rgba(249,115,22,0.4)]` | Botão gradiente menor |
| `electric-card` | Card com glow laranja externo e interno |

CSS do card elétrico:

```css
.electric-card {
  box-shadow:
    0 0 30px rgba(249, 115, 22, 0.3),
    inset 0 0 20px rgba(249, 115, 22, 0.1);
}
```

---

## 4. Gradientes

### 4.1. CTA primário

```txt
bg-gradient-to-t from-yellow-200 via-orange-400 to-orange-500
```

Uso: botão principal de alto destaque.

Características:

- texto escuro: `text-[#2c1306]`
- sombra forte laranja
- ring interno branco translúcido
- hover com escala e sombra maior

### 4.2. CTA terciário / botão de card

```txt
bg-gradient-to-r from-orange-600 via-orange-500 to-amber-500
```

Uso: botão dentro de card, ações secundárias com energia visual.

### 4.3. Texto gradiente

```txt
bg-gradient-to-r from-white via-orange-200 to-orange-400
text-transparent bg-clip-text
```

Uso: métricas, números, headings ou textos especiais.

### 4.4. Borda de card elétrico

```txt
bg-gradient-to-b from-yellow-300 via-orange-500 to-transparent opacity-80
```

Uso: camada externa do card elétrico.

### 4.5. Fundo luminoso de card

```txt
bg-gradient-to-b from-orange-500/10 to-transparent
```

Uso: área superior interna do card, dando leve halo.

### 4.6. Divisor luminoso

```txt
bg-gradient-to-r from-transparent via-orange-500/50 to-transparent
```

Uso: divisória horizontal com centro laranja luminoso.

---

## 5. Tipografia

### 5.1. Escala tipográfica

| Estilo | Classe principal | Tamanho / line-height | Fonte | Peso | Uso |
|---|---|---:|---|---|---|
| Heading 1 | `text-5xl lg:text-[76px] leading-[1.05]` | `76px / ~80px` desktop | Bricolage Grotesque | Light | Hero, títulos principais |
| Heading 2 | `md:text-5xl text-4xl` | `48px / 56px` | Bricolage Grotesque | Light | Seções principais |
| Heading 3 | `text-2xl` | `24px / 32px` | Bricolage Grotesque | Light | Subtítulos de seção |
| Heading 4 | `text-xl` | `20px / 28px` | Inter/sans | Regular | Títulos de cards |
| Body Large | `text-lg leading-relaxed` | `18px / 28px` | Inter/sans | Regular | Parágrafos de introdução |
| Body Regular | `text-sm leading-relaxed` | `14px / 22px` | Inter/sans | Regular | Descrições e textos comuns |
| Small | `text-xs` | `12px / 16px` | Inter/sans | Regular | Labels, menus, specs |
| Micro | `text-[10px]` | ~`10px` | Inter/sans | Regular/uppercase | Badges pequenos |
| Tiny Badge | `text-[11px]` | ~`11px` | Inter/sans | Regular/uppercase | Tag trending |
| Gradient Text | `text-4xl` + gradiente | `36px / 44px` | Bricolage Grotesque | Light | Métricas e destaques |

### 5.2. Regras de headings

Headings grandes devem usar:

```txt
font-bricolage font-light tracking-tight text-white
```

Hero headline:

```txt
text-5xl lg:text-[76px] leading-[1.05] text-white mb-6 font-bricolage font-light tracking-tight
```

Heading 2 de seção:

```txt
md:text-5xl text-4xl text-white font-bricolage font-light tracking-tight mb-4
```

Heading 3:

```txt
text-2xl text-white font-bricolage font-light tracking-tight
```

### 5.3. Regras de corpo

Texto de apoio grande:

```txt
text-lg text-neutral-400 leading-relaxed font-sans
```

Texto comum:

```txt
text-sm text-neutral-400 leading-relaxed font-sans
```

Texto pequeno:

```txt
text-xs text-neutral-400 font-sans
```

Metadados/specs:

```txt
text-xs text-neutral-500 font-mono
```

---

## 6. Estrutura global da página

### 6.1. Body

```html
<body class="min-h-screen overflow-x-hidden selection:bg-orange-500/30 text-white bg-[#050505] relative">
```

Diretrizes:

- altura mínima de tela inteira
- overflow horizontal oculto
- seleção com fundo laranja translúcido
- texto branco por padrão
- fundo preto `#050505`
- layout relativo para camadas de fundo

### 6.2. Background global

O fundo contém três elementos:

1. Campo de estrelas discreto
2. Halo laranja superior central
3. Halo laranja inferior direito

Estrutura:

```html
<div class="fixed z-0 pointer-events-none top-0 right-0 bottom-0 left-0">
  <div class="stars absolute inset-0"></div>
  <div class="absolute top-0 left-1/2 -translate-x-1/2 w-[800px] h-[500px] bg-orange-900/10 blur-[120px] rounded-full"></div>
  <div class="absolute bottom-0 right-0 w-[600px] h-[600px] bg-orange-950/20 blur-[100px] rounded-full"></div>
</div>
```

CSS das estrelas:

```css
.stars {
  background-image:
    radial-gradient(1px 1px at 20px 30px, #fff, rgba(0,0,0,0)),
    radial-gradient(1px 1px at 40px 70px, #ffffff, rgba(0,0,0,0)),
    radial-gradient(1px 1px at 50px 160px, #ffffff, rgba(0,0,0,0)),
    radial-gradient(1.5px 1.5px at 90px 40px, #ffffff, rgba(0,0,0,0)),
    radial-gradient(1px 1px at 130px 80px, #ffffff, rgba(0,0,0,0));
  background-size: 200px 200px;
  opacity: 0.2;
}
```

### 6.3. Grid background para showcase

```css
.grid-bg {
  background-image:
    linear-gradient(to right, #ffffff05 1px, transparent 1px),
    linear-gradient(to bottom, #ffffff05 1px, transparent 1px);
  background-size: 24px 24px;
}
```

Uso: fundos técnicos, áreas de app, demos, dashboards.

### 6.4. Scrollbar oculta

```css
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
```

Uso: carrosséis ou áreas roláveis que precisam manter visual limpo.

---

## 7. Navegação

### 7.1. Navegação sticky do design system

```txt
sticky top-0 z-50 bg-[#050505]/80 backdrop-blur-md border-b border-white/5
```

Container:

```txt
max-w-7xl mx-auto px-6 py-4
```

Links:

```txt
text-xs text-neutral-400 hover:text-white transition-colors font-sans
```

Itens:

- Hero
- Typography
- Colors
- Components
- Layout
- Motion
- Icons

### 7.2. Navegação do hero

Estrutura:

```txt
relative z-50 flex items-center justify-between px-6 py-6 max-w-7xl mx-auto border-b border-white/5 animate-entry delay-75
```

Logo:

- ícone Lucide `asterisk`
- sobreposição de dois asteriscos
- um branco rotacionado `rotate-45`
- um laranja `#f97316`
- tamanho `w-8 h-8` / `32px`
- texto do logo: `text-xl text-white font-sans`

Menu central:

```txt
hidden md:flex items-center bg-white/5 border border-white/10 rounded-full px-1 py-1 backdrop-blur-md
```

Item ativo:

```txt
px-4 py-1.5 bg-neutral-800/80 rounded-full text-xs text-white flex items-center gap-2 border border-white/5 shadow-inner font-sans
```

Indicador ativo:

```txt
w-1.5 h-1.5 rounded-full bg-orange-500
```

Item inativo:

```txt
px-4 py-1.5 text-xs text-neutral-400 hover:text-white transition-colors font-sans
```

CTA no nav:

```txt
hidden md:block hover:brightness-110 transition-all text-sm text-white bg-gradient-to-b from-orange-400 to-orange-600 border-white/20 rounded-full border-t pt-2 pr-5 pb-2 pl-5 shadow-[0_0_15px_-3px_rgba(249,115,22,0.4)] font-sans
```

---

## 8. Hero section

### 8.1. Estrutura geral

```txt
relative z-10
```

Grid principal:

```txt
grid grid-cols-1 lg:grid-cols-12 lg:gap-8 lg:pt-24 max-w-7xl mx-auto pt-16 pr-6 pb-20 pl-6 relative items-center
```

Coluna esquerda:

```txt
lg:col-span-7 flex flex-col relative items-start z-10
```

Coluna direita:

```txt
lg:col-span-5 flex lg:justify-end lg:mt-0 mt-0 relative justify-center z-10 animate-entry delay-500
```

### 8.2. Badge/tag do hero

```txt
mb-8 inline-flex items-center gap-2 px-3 py-1.5 rounded-lg border border-white/10 bg-white/5 backdrop-blur-sm text-xs text-neutral-300 animate-entry delay-100
```

Ícone:

```txt
lucide-sparkles w-3 h-3 text-orange-400
```

Texto original:

```txt
Design System Showcase
```

### 8.3. Headline do hero

Texto original:

```txt
DESIGN SYSTEM
COMPONENTS
PATTERNS STYLES
DOCUMENTATION
```

Classe:

```txt
text-5xl lg:text-[76px] leading-[1.05] text-white mb-6 font-bricolage font-light tracking-tight animate-entry delay-150
```

Diretrizes:

- caixa alta funciona bem nesse sistema
- títulos podem quebrar em múltiplas linhas
- usar tracking tight
- manter peso leve, não usar bold pesado

### 8.4. Subheadline

Texto original:

```txt
A comprehensive guide to the visual language, components, and patterns used throughout the Luminous design system.
```

Classe:

```txt
text-lg text-neutral-400 max-w-xl mb-10 leading-relaxed font-sans animate-entry delay-200
```

### 8.5. Botões do hero

Container:

```txt
flex flex-wrap gap-4 mb-20 items-center animate-entry delay-300
```

Botão principal:

```txt
group relative flex items-center justify-center gap-2.5 rounded-full bg-gradient-to-t from-yellow-200 via-orange-400 to-orange-500 px-8 py-3 text-lg font-medium text-[#2c1306] shadow-[0_0_40px_-5px_rgba(249,115,22,0.6)] ring-1 ring-inset ring-white/40 transition-all duration-300 hover:scale-105 hover:shadow-[0_0_60px_-5px_rgba(249,115,22,0.8)] font-sans
```

Botão secundário:

```txt
px-8 py-3 rounded-full bg-white text-black text-lg hover:bg-neutral-200 transition-colors font-sans
```

Textos originais:

- `Explore Components`
- `View Patterns`

---

## 9. Componentes

## 9.1. Botões

### Botão Primary

Uso: ação principal da tela.

```html
<button class="group relative flex items-center justify-center gap-2.5 rounded-full bg-gradient-to-t from-yellow-200 via-orange-400 to-orange-500 px-8 py-3 text-lg font-medium text-[#2c1306] shadow-[0_0_40px_-5px_rgba(249,115,22,0.6)] ring-1 ring-inset ring-white/40 transition-all duration-300 hover:scale-105 hover:shadow-[0_0_60px_-5px_rgba(249,115,22,0.8)] font-sans">
  <span>Primary</span>
</button>
```

Características:

- formato pill
- gradiente vertical quente
- texto marrom escuro
- sombra laranja forte
- ring interno branco translúcido
- escala no hover

### Botão Secondary

Uso: segunda ação principal, alternativa limpa e de alto contraste.

```html
<button class="px-8 py-3 rounded-full bg-white text-black text-lg hover:bg-neutral-200 transition-colors font-sans">
  Secondary
</button>
```

Características:

- fundo branco
- texto preto
- hover neutro claro
- pill

### Botão Tertiary

Uso: ação dentro de card ou botão energético menos dominante que o Primary.

```html
<button class="hover:brightness-110 transition-all text-sm text-white bg-gradient-to-r from-orange-600 via-orange-500 to-amber-500 border-white/20 rounded-full border-t pt-3 pb-3 pr-5 pl-5 shadow-[0_4px_15px_rgba(249,115,22,0.4)] font-sans">
  Tertiary
</button>
```

Características:

- gradiente horizontal laranja/âmbar
- texto branco
- border-top translúcida
- sombra inferior laranja
- brilho no hover

### Text Button

Uso: ação discreta, links em nav ou opções secundárias.

```html
<button class="px-4 py-1.5 text-xs text-neutral-400 hover:text-white transition-colors font-sans">
  Text Button
</button>
```

---

## 9.2. Cards

### Standard Card

Uso: blocos de conteúdo, documentação, lista de componentes, tiles.

```html
<div class="rounded-xl ring-1 ring-white/10 bg-white/[0.02] p-6 hover:bg-white/[0.04] transition-colors">
  <h4 class="text-sm text-white mb-2 font-sans">Standard Card</h4>
  <p class="text-xs text-neutral-500 font-sans">Card with subtle background and border.</p>
</div>
```

Características:

- fundo quase transparente
- ring branco 10%
- hover levemente mais claro
- padding `p-6`
- raio `rounded-xl`

### Electric Card

Uso: card de destaque, preview, hero visual, métrica principal.

```html
<div class="relative bg-neutral-900 rounded-[32px] p-[2px] electric-card overflow-hidden">
  <div class="absolute inset-0 bg-gradient-to-b from-yellow-300 via-orange-500 to-transparent opacity-80 z-0"></div>
  <div class="relative z-10 bg-[#0A0A0A] rounded-[30px] h-full p-6">
    <h4 class="text-sm text-white mb-2 font-sans">Electric Card</h4>
    <p class="text-xs text-neutral-400 font-sans">Card with glowing border effect.</p>
  </div>
</div>
```

Características:

- container externo `rounded-[32px]`
- padding externo `p-[2px]` para simular borda
- gradiente absoluto como borda luminosa
- interior `#0A0A0A`
- glow via `.electric-card`
- `overflow-hidden`

### Card de preview do hero

Largura:

```txt
w-[360px]
```

Container externo:

```txt
relative w-[360px] bg-neutral-900 rounded-[32px] p-[2px] electric-card overflow-hidden
```

Camada de borda:

```txt
absolute inset-0 bg-gradient-to-b from-yellow-300 via-orange-500 to-transparent opacity-80 z-0
```

Interior:

```txt
relative z-10 bg-[#0A0A0A] rounded-[30px] h-full p-8 flex flex-col items-start overflow-hidden
```

Halo superior interno:

```txt
absolute top-0 right-0 w-full h-40 bg-gradient-to-b from-orange-500/10 to-transparent
```

Badge `Live Preview`:

```txt
text-[10px] uppercase text-neutral-400 border border-white/10 px-2 py-1 rounded bg-white/5 flex items-center gap-1.5 font-sans
```

Indicador pulsante:

```html
<span class="relative flex h-2 w-2">
  <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-400 opacity-75"></span>
  <span class="relative inline-flex rounded-full h-2 w-2 bg-orange-500"></span>
</span>
```

Título:

```txt
text-xl text-white mb-2 font-sans
```

Descrição:

```txt
text-xs text-neutral-400 mb-6 leading-relaxed font-sans
```

Métrica gradiente:

```txt
text-4xl text-transparent bg-clip-text bg-gradient-to-r from-white via-orange-200 to-orange-400 font-bricolage font-light tracking-tight
```

Badge de métrica:

```txt
text-xs text-orange-500 flex items-center bg-orange-500/10 px-1.5 py-0.5 rounded font-sans
```

Botão full width:

```txt
hover:brightness-110 transition-all text-sm text-white bg-gradient-to-r from-orange-600 via-orange-500 to-amber-500 w-full border-white/20 rounded-full border-t mb-8 pt-3 pb-3 shadow-[0_4px_15px_rgba(249,115,22,0.4)] font-sans
```

Divisor:

```txt
relative w-full h-px bg-gradient-to-r from-transparent via-orange-500/50 to-transparent mb-6 flex items-center justify-center
```

Texto sobre divisor:

```txt
bg-[#0A0A0A] px-2 text-[10px] text-neutral-400 uppercase font-sans
```

---

## 9.3. Inputs

### Campo de busca compacto

```html
<div class="hidden sm:flex items-center gap-2 bg-black/40 ring-1 ring-white/10 rounded-lg pl-2.5 pr-2.5 h-8">
  <svg class="lucide lucide-search text-neutral-500" width="14" height="14"></svg>
  <input class="bg-transparent text-sm text-zinc-300 placeholder-zinc-600 focus:outline-none w-48" placeholder="Search..." type="text" />
</div>
```

Características:

- oculto em telas muito pequenas (`hidden sm:flex`)
- altura `h-8`
- fundo preto translúcido
- ring sutil
- ícone `search` em `text-neutral-500`
- input sem borda nem outline

### Input com botão acoplado

```html
<div class="flex items-center rounded-lg bg-[#050505] border border-white/10 focus-within:border-white/20 transition-all">
  <input class="w-full bg-transparent border-none text-sm text-white pl-4 pr-1 py-3 focus:outline-none placeholder:text-neutral-600 font-sans h-10 rounded-l-lg" placeholder="Enter your email" type="email" />
  <button class="bg-white hover:bg-neutral-200 text-black px-4 py-2 rounded-r-lg text-sm transition-colors h-10 shadow-lg flex items-center gap-2 flex-shrink-0 font-sans">
    Submit
  </button>
</div>
```

Características:

- container escuro com borda translúcida
- `focus-within:border-white/20`
- botão branco acoplado à direita
- altura do input e botão: `h-10`

---

## 9.4. Badges e tags

### Badge Live

```html
<span class="text-[10px] uppercase text-neutral-400 border border-white/10 px-2 py-1 rounded bg-white/5 flex items-center gap-1.5 font-sans">
  <span class="relative flex h-2 w-2">
    <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-400 opacity-75"></span>
    <span class="relative inline-flex rounded-full h-2 w-2 bg-orange-500"></span>
  </span>
  Live
</span>
```

Uso: status ativo, ao vivo, preview, sistema funcionando.

### Badge Trending

```html
<span class="text-[11px] rounded-md bg-orange-500/10 text-orange-400 ring-1 ring-orange-500/20 px-2 py-0.5 uppercase font-sans">
  Trending
</span>
```

Uso: item em alta, destaque, oportunidade.

### Tag neutra

```html
<span class="text-[10px] rounded-md bg-white/5 text-neutral-300 ring-1 ring-white/10 px-2 py-0.5 font-sans">
  Tag
</span>
```

Uso: filtros, categorias, labels discretos.

---

## 9.5. Toggles

### Toggle ativo

```html
<button aria-pressed="true" class="relative inline-flex h-5 w-9 items-center rounded-full bg-orange-500/20 ring-1 ring-orange-500/30">
  <span class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-orange-400 translate-x-4 transition-transform shadow-sm"></span>
  <span class="sr-only">toggle</span>
</button>
```

Características:

- fundo laranja translúcido
- ring laranja translúcido
- knob laranja claro
- knob deslocado `translate-x-4`
- acessibilidade com `aria-pressed="true"`

### Toggle inativo

```html
<button aria-pressed="false" class="relative inline-flex h-5 w-9 items-center rounded-full bg-white/10 ring-1 ring-white/10">
  <span class="absolute left-0.5 top-0.5 h-4 w-4 rounded-full bg-neutral-400 translate-x-0 transition-transform shadow-sm"></span>
  <span class="sr-only">toggle</span>
</button>
```

Características:

- fundo branco translúcido
- knob cinza
- knob sem deslocamento
- acessibilidade com `aria-pressed="false"`

---

## 10. Layout e espaçamento

### 10.1. Container padrão

```txt
max-w-7xl mx-auto px-6
```

Uso: largura máxima das seções e navegação.

Exemplo de bloco:

```html
<div class="max-w-7xl mx-auto px-6 py-8 border border-white/10 rounded-xl bg-white/5">
  <div class="text-sm text-white font-sans">max-w-7xl mx-auto px-6</div>
</div>
```

### 10.2. Seções principais

```txt
max-w-7xl mx-auto px-6 py-24 relative z-10
```

Uso: seções de documentação como Typography, Colors, Components, Layout, Motion e Icons.

Cabeçalho de seção:

```txt
mb-16
```

Título de seção:

```txt
md:text-5xl text-4xl text-white font-bricolage font-light tracking-tight mb-4
```

Descrição de seção:

```txt
text-lg text-neutral-400 leading-relaxed font-sans
```

### 10.3. Grid responsivo

Hero:

```txt
grid grid-cols-1 lg:grid-cols-12 lg:gap-8
```

Colunas do hero:

```txt
lg:col-span-7
lg:col-span-5
```

Grid de cores/componentes:

```txt
grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8
```

Grid de cards:

```txt
grid grid-cols-1 md:grid-cols-2 gap-6
```

Grid de ícones:

```txt
grid grid-cols-4 md:grid-cols-8 gap-6
```

### 10.4. Escala de espaçamento

| Classe | Valor | Uso |
|---|---:|---|
| `gap-1` | 4px | microespaços internos |
| `gap-1.5` | 6px | badges com ícones pequenos |
| `gap-2` | 8px | logo, nav, grupos pequenos |
| `gap-2.5` | 10px | botões principais com ícone |
| `gap-3` | 12px | listas compactas |
| `gap-4` | 16px | botões e elementos médios |
| `gap-6` | 24px | cards em grids |
| `gap-8` | 32px | grids principais |
| `px-2` | 8px | tags pequenas |
| `px-3` | 12px | badge hero |
| `px-4` | 16px | nav links e botões pequenos |
| `px-6` | 24px | container |
| `px-8` | 32px | botões grandes |
| `py-0.5` | 2px | micro tags |
| `py-1` | 4px | badges |
| `py-1.5` | 6px | nav items |
| `py-3` | 12px | botões grandes |
| `py-4` | 16px | nav sticky |
| `py-6` | 24px | nav hero |
| `py-8` | 32px | containers demonstrativos |
| `py-24` | 96px | seções principais |

---

## 11. Motion e interação

### 11.1. Animação de entrada: `fadeInUpBlur`

CSS:

```css
@keyframes fadeInUpBlur {
  0% {
    opacity: 0;
    transform: translateY(20px);
    filter: blur(10px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    filter: blur(0);
  }
}

.animate-entry {
  animation: fadeInUpBlur 1s cubic-bezier(0.2,0.8,0.2,1) both;
}
```

Delays disponíveis:

```css
.delay-75  { animation-delay: 75ms; }
.delay-100 { animation-delay: 100ms; }
.delay-150 { animation-delay: 150ms; }
.delay-200 { animation-delay: 200ms; }
.delay-300 { animation-delay: 300ms; }
.delay-500 { animation-delay: 500ms; }
.delay-700 { animation-delay: 700ms; }
```

Uso:

```txt
animate-entry delay-100
animate-entry delay-150
animate-entry delay-200
animate-entry delay-300
animate-entry delay-500
```

### 11.2. Animação alternativa: `animationIn`

CSS:

```css
@keyframes animationIn {
  0% {
    opacity: 0;
    transform: translateY(30px);
    filter: blur(8px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    filter: blur(0px);
  }
}
```

Classe Tailwind arbitrária usada:

```txt
[animation:animationIn_0.8s_ease-out_0.1s_both]
```

### 11.3. Animação no scroll

Comportamento:

- elementos `.animate-on-scroll` começam pausados
- quando entram na viewport, recebem classe `.animate`
- animação roda uma única vez
- threshold: `0.2`
- root margin: `0px 0px -10% 0px`

CSS injetado:

```css
.animate-on-scroll {
  animation-play-state: paused !important;
}

.animate-on-scroll.animate {
  animation-play-state: running !important;
}
```

JavaScript:

```js
(function () {
  const style = document.createElement("style");
  style.textContent = `
    .animate-on-scroll { animation-play-state: paused !important; }
    .animate-on-scroll.animate { animation-play-state: running !important; }
  `;
  document.head.appendChild(style);

  const once = true;

  if (!window.__inViewIO) {
    window.__inViewIO = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          entry.target.classList.add("animate");
          if (once) window.__inViewIO.unobserve(entry.target);
        }
      });
    }, { threshold: 0.2, rootMargin: "0px 0px -10% 0px" });
  }

  window.initInViewAnimations = function (selector = ".animate-on-scroll") {
    document.querySelectorAll(selector).forEach((el) => {
      window.__inViewIO.observe(el);
    });
  };

  document.addEventListener("DOMContentLoaded", () => initInViewAnimations());
})();
```

### 11.4. Hover effects

| Classe | Comportamento |
|---|---|
| `hover:bg-white/[0.04] transition-colors` | clareia levemente o card |
| `hover:border-white/20 transition-colors` | destaca borda |
| `hover:scale-105 transition-transform` | aumenta discretamente elemento |
| `hover:brightness-110 transition-all` | aumenta brilho de CTA |
| `hover:text-white transition-colors` | link inativo fica branco |
| `hover:bg-neutral-200 transition-colors` | botão branco escurece levemente |

Regra: hover deve ser perceptível, mas discreto. Evitar transformações agressivas.

---

## 12. Sistema de ícones

Biblioteca usada: **Lucide Icons**.

Script original:

```html
<script src="assets/lucide_latest_2eebd0ebe8c2.js"></script>
<script>
  lucide.createIcons();
</script>
```

### 12.1. Tamanhos

| Classe | Tamanho | Uso |
|---|---:|---|
| `w-3 h-3` | 12px | ícones dentro de badges pequenas |
| `w-3.5 h-3.5` | 14px | microícones em rodapés/tags |
| `w-4 h-4` | 16px | ícones de listas e métricas |
| `w-6 h-6` | 24px | ícones comuns em grids e seções |
| `w-8 h-8` | 32px | logo/asterisco principal |
| `w-[32px] h-[32px]` | 32px | sobreposição exata do logo |

### 12.2. Cores de ícone

| Classe | Uso |
|---|---|
| `text-white` | ícone padrão de alto contraste |
| `text-neutral-400` | ícone secundário |
| `text-neutral-500` | ícone auxiliar em input |
| `text-orange-400` | destaque quente suave |
| `text-orange-500` | destaque/ação principal |
| `fill-white` | ícone preenchido em tag especial |

### 12.3. Ícones comuns

| Ícone Lucide | Uso recomendado |
|---|---|
| `asterisk` | marca, símbolo visual, logo abstrato |
| `sparkles` | destaque, magia, AI, feature especial |
| `zap` | energia, velocidade, automação |
| `trending-up` | crescimento, métrica, resultado positivo |
| `eye` | visualização, componentes, preview |
| `search` | busca |
| `chevron-down` | dropdown, menu expansível |
| `arrow-up` | crescimento, incremento, cobertura |

### 12.4. Logo com asterisco sobreposto

Estrutura visual:

- container relativo `w-8 h-8`
- primeiro asterisco branco, absoluto, rotacionado `rotate-45`
- segundo asterisco laranja, absoluto, tamanho `32px`

Uso recomendado:

```html
<div class="relative flex items-center justify-center w-8 h-8">
  <i data-lucide="asterisk" class="text-white w-8 h-8 absolute rotate-45"></i>
  <i data-lucide="asterisk" class="absolute w-[32px] h-[32px] text-orange-500"></i>
</div>
```

---

## 13. Gráfico sparkline do card

O card de preview contém um gráfico SVG com área em gradiente e linha laranja.

### 13.1. SVG base

```html
<svg class="w-full h-full overflow-visible" preserveAspectRatio="none" viewBox="0 0 280 60">
  <defs>
    <linearGradient id="chartGradient" x1="0" x2="0" y1="0" y2="1">
      <stop offset="0%" stop-color="#f97316" stop-opacity="0.3"></stop>
      <stop offset="100%" stop-color="#f97316" stop-opacity="0"></stop>
    </linearGradient>
  </defs>
  <path d="M0 50 C 40 50, 60 30, 100 35 C 140 40, 160 10, 200 15 C 240 20, 260 5, 280 0 V 60 H 0 Z" fill="url(#chartGradient)"></path>
  <path d="M0 50 C 40 50, 60 30, 100 35 C 140 40, 160 10, 200 15 C 240 20, 260 5, 280 0" fill="none" stroke="#f97316" stroke-linecap="round" stroke-width="2" vector-effect="non-scaling-stroke"></path>
  <circle cx="280" cy="0" fill="#fff" r="3" stroke="#f97316" stroke-width="2"></circle>
</svg>
```

### 13.2. Diretrizes

- usar linha laranja `#f97316`
- área inferior com opacidade de 0.3 no topo e 0 no final
- ponto final branco com stroke laranja
- manter `stroke-linecap="round"`
- usar `vector-effect="non-scaling-stroke"`

---

## 14. Seções do design system original

O HTML original possui as seguintes seções navegáveis:

1. `#hero` — Hero Section / demonstração visual principal
2. `#typography` — escala tipográfica
3. `#colors` — cores e superfícies
4. `#components` — componentes de UI
5. `#layout` — layout e espaçamento
6. `#motion` — motion e interação
7. `#icons` — sistema de ícones

Estrutura padrão de seção:

```html
<section id="section-id" class="max-w-7xl mx-auto px-6 py-24 relative z-10">
  <div class="mb-16">
    <h2 class="md:text-5xl text-4xl text-white font-bricolage font-light tracking-tight mb-4">Section Title</h2>
    <p class="text-lg text-neutral-400 leading-relaxed font-sans">Section description.</p>
  </div>
  <!-- conteúdo -->
</section>
```

---

## 15. Diretrizes para geração por IA

### 15.1. Prompt visual compacto

Use este prompt quando quiser que uma IA gere uma interface seguindo este design system:

```txt
Create a premium dark SaaS interface using the InovaSala / Luminous design system: deep black background #050505, card surfaces #0A0A0A and neutral-900, subtle white translucent borders, orange/amber accent color #f97316, warm orange glows, electric gradient borders, soft radial background halos, Bricolage Grotesque light headings, Inter body text, rounded pill buttons, glassmorphism overlays, Lucide line icons, minimal high-contrast layout, elegant futuristic component library aesthetic. Use orange only as accent and CTA energy, not as the main background.
```

### 15.2. Prompt para gerar landing page

```txt
Generate a responsive landing page using the InovaSala / Luminous visual system. Use a #050505 background with subtle star particles and blurred orange halos. Use max-w-7xl containers with px-6 and py-24 sections. Headings must use Bricolage Grotesque, font-light, tracking-tight, white text. Body copy must use Inter, neutral-400, relaxed line-height. CTAs should use rounded-full shapes, orange/yellow gradients, warm glow shadows, and subtle hover scale. Cards should use dark surfaces, white/10 rings, white/5 overlays, and optional electric orange gradient borders. Icons should be Lucide line icons in white, neutral-400, orange-400, or orange-500.
```

### 15.3. Prompt para gerar componentes

```txt
Build UI components in Tailwind using this design language: dark base #050505, card interior #0A0A0A, subtle rings border-white/10, hover:bg-white/[0.04], orange accent #f97316, rounded-xl cards, rounded-full buttons, Bricolage Grotesque light headings, Inter body text, Lucide icons, soft orange glows, and restrained transitions. Components should feel premium, minimal, futuristic, and highly readable.
```

### 15.4. Prompt negativo

```txt
Do not use bright white backgrounds, saturated multicolor palettes, heavy borders, flat corporate blue, childish illustration, excessive neon, thick shadows, low contrast text, rounded cartoon UI, dense visual clutter, or orange as a full-page background. Avoid bold heavy headings; use light elegant typography.
```

---

## 16. Regras práticas de uso

### 16.1. Faça

- Usar `#050505` como fundo dominante.
- Usar `#0A0A0A` para interiores de cards premium.
- Usar `orange-500` como acento principal.
- Usar headings grandes, leves e com tracking tight.
- Usar `max-w-7xl mx-auto px-6` como container base.
- Usar bordas `white/5` e `white/10`.
- Usar blur e gradientes com baixa opacidade.
- Usar botões `rounded-full`.
- Usar `Lucide` para ícones lineares.
- Usar animações suaves de entrada com translateY e blur.

### 16.2. Não faça

- Não trocar a base escura por fundo claro.
- Não usar laranja como cor de fundo massiva.
- Não usar gradientes muito coloridos ou fora da paleta quente.
- Não usar fontes pesadas nos headings principais.
- Não usar bordas sólidas e claras demais.
- Não exagerar no glow.
- Não usar muitos efeitos animados simultâneos.
- Não misturar ícones preenchidos de estilos diferentes, exceto casos pontuais como `fill-white`.
- Não reduzir o contraste do texto principal.

---

## 17. Tokens em formato JSON para IA

```json
{
  "name": "InovaSala / Luminous Design System",
  "source_display_name": "Luminous",
  "visual_style": [
    "dark premium SaaS",
    "futuristic",
    "minimal",
    "orange glow",
    "electric border",
    "high contrast",
    "glassmorphism subtle"
  ],
  "fonts": {
    "body": "Inter, sans-serif",
    "display": "Bricolage Grotesque, sans-serif",
    "mono": "monospace"
  },
  "colors": {
    "background_primary": "#050505",
    "background_card": "#0A0A0A",
    "surface": "neutral-900",
    "text_primary": "#ffffff",
    "text_secondary": "neutral-300",
    "text_tertiary": "neutral-400",
    "text_muted": "neutral-500",
    "accent_primary": "#f97316",
    "accent_light": "#fb923c",
    "accent_deep": "#ea580c",
    "highlight": "#f59e0b",
    "warm_light": "#fef08a"
  },
  "typography": {
    "h1": "text-5xl lg:text-[76px] leading-[1.05] font-bricolage font-light tracking-tight",
    "h2": "md:text-5xl text-4xl font-bricolage font-light tracking-tight",
    "h3": "text-2xl font-bricolage font-light tracking-tight",
    "h4": "text-xl font-sans",
    "body_large": "text-lg leading-relaxed font-sans text-neutral-400",
    "body": "text-sm leading-relaxed font-sans text-neutral-400",
    "small": "text-xs font-sans text-neutral-400",
    "micro": "text-[10px] font-sans"
  },
  "layout": {
    "container": "max-w-7xl mx-auto px-6",
    "section": "max-w-7xl mx-auto px-6 py-24 relative z-10",
    "hero_grid": "grid grid-cols-1 lg:grid-cols-12 lg:gap-8",
    "hero_left": "lg:col-span-7",
    "hero_right": "lg:col-span-5",
    "standard_grid": "grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8"
  },
  "components": {
    "primary_button": "rounded-full bg-gradient-to-t from-yellow-200 via-orange-400 to-orange-500 px-8 py-3 text-lg font-medium text-[#2c1306] shadow-[0_0_40px_-5px_rgba(249,115,22,0.6)] ring-1 ring-inset ring-white/40 hover:scale-105",
    "secondary_button": "rounded-full bg-white text-black px-8 py-3 text-lg hover:bg-neutral-200",
    "tertiary_button": "rounded-full bg-gradient-to-r from-orange-600 via-orange-500 to-amber-500 text-white shadow-[0_4px_15px_rgba(249,115,22,0.4)] hover:brightness-110",
    "standard_card": "rounded-xl ring-1 ring-white/10 bg-white/[0.02] p-6 hover:bg-white/[0.04]",
    "electric_card": "relative bg-neutral-900 rounded-[32px] p-[2px] electric-card overflow-hidden",
    "input": "bg-[#050505] border border-white/10 focus-within:border-white/20 rounded-lg",
    "badge_live": "text-[10px] uppercase text-neutral-400 border border-white/10 px-2 py-1 rounded bg-white/5 flex items-center gap-1.5",
    "toggle_on": "h-5 w-9 rounded-full bg-orange-500/20 ring-1 ring-orange-500/30",
    "toggle_off": "h-5 w-9 rounded-full bg-white/10 ring-1 ring-white/10"
  },
  "motion": {
    "entry": "fadeInUpBlur 1s cubic-bezier(0.2,0.8,0.2,1) both",
    "scroll_entry": "animationIn 0.8s ease-out 0.1s both paused until visible",
    "delays": ["75ms", "100ms", "150ms", "200ms", "300ms", "500ms", "700ms"],
    "hover": ["hover:bg-white/[0.04]", "hover:border-white/20", "hover:scale-105", "hover:brightness-110", "hover:text-white"]
  },
  "icons": {
    "library": "Lucide",
    "common": ["asterisk", "sparkles", "zap", "trending-up", "eye", "search", "chevron-down", "arrow-up"],
    "sizes": ["w-3 h-3", "w-4 h-4", "w-6 h-6", "w-8 h-8"],
    "colors": ["text-white", "text-neutral-400", "text-orange-400", "text-orange-500"]
  }
}
```

---

## 18. Checklist de consistência para outputs de IA

Antes de aceitar uma tela gerada com este design system, verificar:

- [ ] O fundo principal é `#050505` ou equivalente muito escuro.
- [ ] Os cards usam `#0A0A0A`, `neutral-900` ou overlays `white/[0.02]`.
- [ ] O laranja aparece como acento, não como massa de fundo.
- [ ] Headings usam Bricolage Grotesque, peso leve e tracking tight.
- [ ] Parágrafos usam texto `neutral-400` com line-height confortável.
- [ ] Botões principais são pills com gradiente quente e glow.
- [ ] Bordas são translúcidas (`white/5`, `white/10`, `white/20`).
- [ ] Ícones são Lucide, lineares e consistentes.
- [ ] Hover states são sutis.
- [ ] Animações são suaves, com blur/translate e delays escalonados.
- [ ] O layout usa `max-w-7xl mx-auto px-6`.
- [ ] O visual final parece premium, tecnológico e limpo.

---

## 19. Snippet base para nova página

```html
<body class="min-h-screen overflow-x-hidden selection:bg-orange-500/30 text-white bg-[#050505] relative">
  <div class="fixed z-0 pointer-events-none inset-0">
    <div class="stars absolute inset-0"></div>
    <div class="absolute top-0 left-1/2 -translate-x-1/2 w-[800px] h-[500px] bg-orange-900/10 blur-[120px] rounded-full"></div>
    <div class="absolute bottom-0 right-0 w-[600px] h-[600px] bg-orange-950/20 blur-[100px] rounded-full"></div>
  </div>

  <section class="max-w-7xl mx-auto px-6 py-24 relative z-10">
    <div class="mb-16">
      <h2 class="md:text-5xl text-4xl text-white font-bricolage font-light tracking-tight mb-4">
        Section Title
      </h2>
      <p class="text-lg text-neutral-400 leading-relaxed font-sans">
        Section description goes here.
      </p>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
      <div class="rounded-xl ring-1 ring-white/10 bg-white/[0.02] p-6 hover:bg-white/[0.04] transition-colors">
        <h3 class="text-xl text-white mb-2 font-sans">Card title</h3>
        <p class="text-sm text-neutral-400 leading-relaxed font-sans">Card copy.</p>
      </div>
    </div>
  </section>
</body>
```

---

## 20. CSS utilitário essencial

```css
body {
  font-family: 'Inter', sans-serif;
  background-color: #050505;
}

.font-bricolage {
  font-family: 'Bricolage Grotesque', sans-serif !important;
}

.electric-card {
  box-shadow:
    0 0 30px rgba(249, 115, 22, 0.3),
    inset 0 0 20px rgba(249, 115, 22, 0.1);
}

.electric-border {
  background: linear-gradient(180deg, rgba(251, 146, 60, 1) 0%, rgba(251, 146, 60, 0) 100%);
}

.stars {
  background-image:
    radial-gradient(1px 1px at 20px 30px, #fff, rgba(0,0,0,0)),
    radial-gradient(1px 1px at 40px 70px, #ffffff, rgba(0,0,0,0)),
    radial-gradient(1px 1px at 50px 160px, #ffffff, rgba(0,0,0,0)),
    radial-gradient(1.5px 1.5px at 90px 40px, #ffffff, rgba(0,0,0,0)),
    radial-gradient(1px 1px at 130px 80px, #ffffff, rgba(0,0,0,0));
  background-size: 200px 200px;
  opacity: 0.2;
}

.grid-bg {
  background-image:
    linear-gradient(to right, #ffffff05 1px, transparent 1px),
    linear-gradient(to bottom, #ffffff05 1px, transparent 1px);
  background-size: 24px 24px;
}

.no-scrollbar::-webkit-scrollbar {
  display: none;
}

.no-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

@keyframes fadeInUpBlur {
  0% {
    opacity: 0;
    transform: translateY(20px);
    filter: blur(10px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    filter: blur(0);
  }
}

.animate-entry {
  animation: fadeInUpBlur 1s cubic-bezier(0.2,0.8,0.2,1) both;
}

.delay-75 { animation-delay: 75ms; }
.delay-100 { animation-delay: 100ms; }
.delay-150 { animation-delay: 150ms; }
.delay-200 { animation-delay: 200ms; }
.delay-300 { animation-delay: 300ms; }
.delay-500 { animation-delay: 500ms; }
.delay-700 { animation-delay: 700ms; }

@keyframes animationIn {
  0% {
    opacity: 0;
    transform: translateY(30px);
    filter: blur(8px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
    filter: blur(0px);
  }
}
```
