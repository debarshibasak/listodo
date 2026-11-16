import * as React from "react"
import { ContextMenu as Primitive } from "radix-ui"
import { cn } from "@/lib/utils"

export const ContextMenu = Primitive.Root
export const ContextMenuTrigger = Primitive.Trigger

export function ContextMenuContent({ className, ...props }: React.ComponentProps<typeof Primitive.Content>) {
  return <Primitive.Portal>
    <Primitive.Content data-no-focus-ring {...props} className={cn(
      "z-50 min-w-48 overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md", className,
    )} />
  </Primitive.Portal>
}

export function ContextMenuItem({ className, ...props }: React.ComponentProps<typeof Primitive.Item>) {
  return <Primitive.Item data-no-focus-ring {...props} className={cn(
    "flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:size-4", className,
  )} />
}

export function ContextMenuLabel(props: React.ComponentProps<typeof Primitive.Label>) {
  return <Primitive.Label className="px-2 py-1.5 text-xs text-muted-foreground" {...props} />
}
