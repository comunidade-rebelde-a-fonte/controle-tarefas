import { cva } from "class-variance-authority";

/**
 * Botões do Design System InovaSala (TASK-029): em pílula; o principal com o gradiente quente do
 * material, texto escuro e um brilho laranja curto; o secundário claro; os demais discretos, com
 * borda fina. O de perigo usa o vermelho de estado em contorno.
 */
export const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-full text-sm font-medium whitespace-nowrap transition-all focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default:
          "bg-gradient-to-t from-yellow-200 via-orange-400 to-orange-500 text-primary-foreground shadow-[0_0_15px_-3px_rgba(249,115,22,0.4)] ring-1 ring-white/40 ring-inset hover:brightness-110",
        secondary: "bg-white text-black hover:bg-neutral-200",
        outline: "border border-white/10 bg-white/[0.02] text-neutral-300 hover:bg-white/5 hover:text-white",
        ghost: "text-neutral-400 hover:bg-white/5 hover:text-white",
        destructive: "border border-destructive/40 bg-destructive/10 text-destructive hover:bg-destructive/15",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 px-3.5",
        sm: "h-8 px-2.5 text-xs",
        lg: "h-10 px-5",
        icon: "size-8",
        "icon-sm": "size-7",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);
