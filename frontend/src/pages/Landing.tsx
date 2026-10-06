import { Link } from 'react-router-dom'
import { motion, useReducedMotion } from 'motion/react'
import { Button } from '../components/ui/Button'

function RevealStagger({ children, delayOffset = 0 }: { children: React.ReactNode, delayOffset?: number }) {
  const reduce = useReducedMotion()
  return (
    <motion.div
      initial={reduce ? false : { opacity: 0, y: 16 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.1 }}
      transition={{
        duration: 0.5,
        delay: delayOffset,
        ease: [0.16, 1, 0.3, 1],
      }}
    >
      {children}
    </motion.div>
  )
}

export function Landing() {
  return (
    <div className="flex-1 bg-surface font-sans">
      
      {/* 1. HERO SECTION (Split Screen) */}
      <section className="max-w-[1400px] mx-auto px-6 pt-16 md:pt-24 pb-16">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-12 lg:gap-8 items-center">
          
          {/* Left: Copy & CTAs */}
          <div className="max-w-xl">
            <RevealStagger>
              <h1 className="text-4xl md:text-5xl lg:text-6xl font-bold tracking-tighter text-slate-900 leading-[1.05]">
                Clinical intelligence, built for both sides of care.
              </h1>
            </RevealStagger>
            
            <RevealStagger delayOffset={0.1}>
              <p className="mt-6 text-lg md:text-xl text-slate-600 leading-relaxed max-w-[50ch]">
                A unified platform connecting patient health data directly to clinical workflows without sacrificing security or consumer experience.
              </p>
            </RevealStagger>

            <RevealStagger delayOffset={0.2}>
              <div className="mt-8 flex flex-col sm:flex-row gap-4">
                <Link to="/login?role=patient" className="w-full sm:w-auto">
                  <Button size="lg" isPatientTheme className="w-full">
                    For Patients
                  </Button>
                </Link>
                <Link to="/login?role=doctor" className="w-full sm:w-auto">
                  <Button size="lg" variant="outline" className="w-full">
                    For Doctors & Clinics
                  </Button>
                </Link>
              </div>
            </RevealStagger>
          </div>

          {/* Right: Premium Generated Asset */}
          <RevealStagger delayOffset={0.15}>
            <div className="relative aspect-square md:aspect-[4/3] lg:aspect-square w-full max-w-lg mx-auto lg:ml-auto overflow-hidden rounded-2xl bg-slate-100 border border-border shadow-sm">
              <img 
                src="/hero.jpg" 
                alt="Abstract 3D representation of connected healthcare data" 
                className="object-cover w-full h-full mix-blend-multiply"
              />
            </div>
          </RevealStagger>
        </div>
      </section>

      {/* 2. LOGO WALL (Social Proof) */}
      <section className="border-y border-border bg-white py-12">
        <div className="max-w-5xl mx-auto px-6 flex flex-col md:flex-row justify-between items-center gap-8 opacity-60 grayscale">
          {/* SVGs representing tech/compliance or fictional partners */}
          <svg height="32" viewBox="0 0 100 30" fill="currentColor" className="text-slate-900"><path d="M10,15 L20,5 L30,15 L20,25 Z M35,15 A5,5 0 1,1 45,15 A5,5 0 1,1 35,15 M50,5 h10 v20 h-10 Z M65,25 l5,-20 l5,20 Z" /></svg>
          <svg height="30" viewBox="0 0 120 30" fill="currentColor" className="text-slate-900"><rect x="0" y="5" width="20" height="20" rx="4" /><rect x="25" y="5" width="20" height="20" rx="10" /><path d="M55,25 L65,5 L75,25 Z" /><circle cx="95" cy="15" r="10" /></svg>
          <svg height="28" viewBox="0 0 90 30" fill="currentColor" className="text-slate-900"><path d="M5,5 h20 v5 h-15 v5 h10 v5 h-10 v5 h15 v5 h-20 Z M35,5 h5 v25 h-5 Z M50,5 h20 v5 h-15 v15 h15 v5 h-20 Z" /></svg>
          <svg height="26" viewBox="0 0 100 30" fill="currentColor" className="text-slate-900"><circle cx="15" cy="15" r="12" fill="none" stroke="currentColor" strokeWidth="4" /><rect x="35" y="3" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="4" /><polygon points="75,27 87,3 99,27" fill="none" stroke="currentColor" strokeWidth="4" /></svg>
        </div>
      </section>

      {/* 3. BENTO GRID (Feature Architecture) */}
      <section className="max-w-[1400px] mx-auto px-6 py-24">
        <RevealStagger>
          <h2 className="text-3xl font-bold tracking-tight text-slate-900 mb-10 max-w-xl">
            A secure bridge between consumer ease and clinical rigor.
          </h2>
        </RevealStagger>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {/* Cell 1: Large Asymmetric feature */}
          <RevealStagger delayOffset={0.1}>
            <div className="md:col-span-2 bg-brand-50 rounded-2xl p-8 md:p-10 border border-brand-100 flex flex-col justify-between h-full min-h-[300px]">
              <div className="max-w-md">
                <h3 className="text-2xl font-bold text-brand-900 mb-3">AI-Powered OCR Verification</h3>
                <p className="text-brand-700 leading-relaxed">
                  Upload physical prescriptions. Our dual-engine LLM extracts, structures, and flags drug interactions instantly for doctor review.
                </p>
              </div>
              <div className="mt-8 flex gap-2">
                <span className="bg-white text-brand-700 text-xs font-bold px-3 py-1 rounded shadow-sm border border-brand-100">GPT-4o</span>
                <span className="bg-white text-brand-700 text-xs font-bold px-3 py-1 rounded shadow-sm border border-brand-100">Claude 3.5</span>
              </div>
            </div>
          </RevealStagger>

          {/* Cell 2: Standard cell */}
          <RevealStagger delayOffset={0.2}>
            <div className="bg-white rounded-2xl p-8 border border-border flex flex-col justify-between h-full min-h-[300px] shadow-sm">
              <div>
                <h3 className="text-xl font-bold text-slate-900 mb-3">Longitudinal Vitals</h3>
                <p className="text-slate-600 leading-relaxed">
                  Patients log vitals in a warm, simple interface. Doctors see a dense, sortable data grid.
                </p>
              </div>
              <div className="mt-8">
                <div className="flex justify-between items-end border-b border-slate-100 pb-2">
                  <span className="font-sans font-semibold text-slate-800">Heart Rate</span>
                  <span className="font-mono text-red-600 font-bold">145 bpm</span>
                </div>
              </div>
            </div>
          </RevealStagger>
          
          {/* Cell 3: Standard cell */}
          <RevealStagger delayOffset={0.3}>
            <div className="bg-slate-900 rounded-2xl p-8 flex flex-col justify-between h-full min-h-[300px] shadow-lg">
              <div>
                <h3 className="text-xl font-bold text-white mb-3">Safety Interrupts</h3>
                <p className="text-slate-400 leading-relaxed">
                  Built-in OpenFDA integration automatically flags critical interactions before they reach the pharmacy.
                </p>
              </div>
              <div className="mt-8 flex items-center gap-3 bg-red-500/10 border border-red-500/20 p-3 rounded-lg">
                <div className="w-2 h-2 rounded-full bg-red-500 animate-pulse"></div>
                <span className="text-red-400 font-mono text-xs uppercase font-bold tracking-wider">Alert Triggered</span>
              </div>
            </div>
          </RevealStagger>
        </div>
      </section>

    </div>
  )
}
