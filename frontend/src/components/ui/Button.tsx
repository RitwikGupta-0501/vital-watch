import React from 'react'
import { cn } from '../../lib/utils'

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'outline' | 'ghost'
  size?: 'sm' | 'md' | 'lg'
  isPatientTheme?: boolean
}

export function Button({ 
  className, 
  variant = 'primary', 
  size = 'md', 
  isPatientTheme = false,
  ...props 
}: ButtonProps) {
  const baseStyles = "inline-flex items-center justify-center font-medium transition-all focus:outline-none disabled:opacity-50 disabled:pointer-events-none"
  
  // Patient theme vs Doctor theme base styles
  const themeStyles = isPatientTheme 
    ? "rounded-full active:scale-[0.98]" 
    : "rounded-md active:translate-y-[1px]"
    
  const sizeStyles = {
    sm: "px-3 py-1.5 text-sm",
    md: "px-4 py-2 text-sm",
    lg: "px-6 py-3 text-lg"
  }
  
  const variantStyles = {
    primary: "bg-brand-700 text-white hover:bg-brand-800 shadow-sm",
    outline: "border border-border bg-white text-slate-700 hover:bg-slate-50 shadow-sm",
    ghost: "bg-transparent text-slate-700 hover:bg-slate-100"
  }

  return (
    <button 
      className={cn(baseStyles, themeStyles, sizeStyles[size], variantStyles[variant], className)} 
      {...props} 
    />
  )
}
