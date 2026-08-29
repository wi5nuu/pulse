// E.112: Special characters dialog untuk insert simbol spesial
'use client'

import { useState } from 'react'

const SPECIAL_CHAR_CATEGORIES = {
  currency: ['$', '€', '£', '¥', '₹', '₽', '₩', '₪', '₱', '₫', '₦', '₴', '₡', '₵', '₸', '₺', '₼', '₾', '₿'],
  math: ['±', '×', '÷', '=', '≠', '≈', '≡', '≤', '≥', '<', '>', '∞', '∫', '∑', '∏', '√', '∛', '∜', 'π', '∂', '∆', '∇', '∈', '∉', '∋', '∀', '∃', '∄', '∅', '∩', '∪', '⊂', '⊃', '⊆', '⊇', '⊕', '⊗', '⊥', '∥', '∠', '∟', '°', '′', '″', '∴', '∵', '∼', '≅', '≃', '≄', '≪', '≫'],
  arrows: ['←', '→', '↑', '↓', '↔', '↕', '↖', '↗', '↘', '↙', '⇐', '⇒', '⇑', '⇓', '⇔', '⇕', '⇖', '⇗', '⇘', '⇙', '⇤', '⇥', '⇦', '⇧', '⇨', '⇩', '⇪', '↩', '↪', '↫', '↬', '↭', '↯', '↰', '↱', '↲', '↳', '↴', '↵', '↶', '↷', '↸', '↹', '↺', '↻'],
  punctuation: ['‐', '–', '—', '―', '‖', ''', ''', '‚', '‛', '"', '"', '„', '‟', '•', '‣', '․', '‥', '…', '‧', '′', '″', '‴', '‵', '‶', '‷', '‹', '›', '※', '‼', '‽', '⁇', '⁈', '⁉', '⁎', '⁏', '⁐', '⁑'],
  greek: ['Α', 'Β', 'Γ', 'Δ', 'Ε', 'Ζ', 'Η', 'Θ', 'Ι', 'Κ', 'Λ', 'Μ', 'Ν', 'Ξ', 'Ο', 'Π', 'Ρ', 'Σ', 'Τ', 'Υ', 'Φ', 'Χ', 'Ψ', 'Ω', 'α', 'β', 'γ', 'δ', 'ε', 'ζ', 'η', 'θ', 'ι', 'κ', 'λ', 'μ', 'ν', 'ξ', 'ο', 'π', 'ρ', 'σ', 'τ', 'υ', 'φ', 'χ', 'ψ', 'ω'],
  latin: ['À', 'Á', 'Â', 'Ã', 'Ä', 'Å', 'Æ', 'Ç', 'È', 'É', 'Ê', 'Ë', 'Ì', 'Í', 'Î', 'Ï', 'Ð', 'Ñ', 'Ò', 'Ó', 'Ô', 'Õ', 'Ö', 'Ø', 'Ù', 'Ú', 'Û', 'Ü', 'Ý', 'Þ', 'ß', 'à', 'á', 'â', 'ã', 'ä', 'å', 'æ', 'ç', 'è', 'é', 'ê', 'ë', 'ì', 'í', 'î', 'ï', 'ð', 'ñ', 'ò', 'ó', 'ô', 'õ', 'ö', 'ø', 'ù', 'ú', 'û', 'ü', 'ý', 'þ', 'ÿ'],
  symbols: ['©', '®', '™', '℗', '℠', '§', '¶', '†', '‡', '‰', '‱', '′', '″', '№', '℃', '℉', '℧', 'Ω', '℮', '⅐', '⅑', '⅒', '⅓', '⅔', '⅕', '⅖', '⅗', '⅘', '⅙', '⅚', '⅛', '⅜', '⅝', '⅞', '¼', '½', '¾', '⅟', '↉'],
  box: ['─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼', '═', '║', '╒', '╓', '╔', '╕', '╖', '╗', '╘', '╙', '╚', '╛', '╜', '╝', '╞', '╟', '╠', '╡', '╢', '╣', '╤', '╥', '╦', '╧', '╨', '╩', '╪', '╫', '╬', '╭', '╮', '╯', '╰', '╱', '╲', '╳'],
}

const CHAR_NAMES: Record<string, string> = {
  '$': 'dollar money',
  '€': 'euro currency',
  '£': 'pound currency',
  '¥': 'yen currency',
  '₹': 'rupee currency',
  '₽': 'ruble currency',
  '₩': 'won currency',
  '₿': 'bitcoin crypto',
  '±': 'plus-minus plus or minus',
  '×': 'multiplication times',
  '÷': 'division divide',
  '≠': 'not equal',
  '≈': 'approximately equal',
  '≡': 'identical equivalent',
  '≤': 'less than or equal',
  '≥': 'greater than or equal',
  '∞': 'infinity',
  'π': 'pi greek',
  '∑': 'summation sum sigma',
  '∏': 'product pi',
  '√': 'square root radical',
  '∆': 'delta increment change',
  '∇': 'nabla gradient',
  '∈': 'element of member',
  '∉': 'not element of',
  '∀': 'for all universal',
  '∃': 'exists there exists',
  '∅': 'empty set null',
  '∩': 'intersection set',
  '∪': 'union set',
  '⊂': 'subset proper',
  '⊃': 'superset proper',
  '⊆': 'subset or equal',
  '⊇': 'superset or equal',
  '⊕': 'direct sum xor',
  '⊗': 'tensor product',
  '⊥': 'perpendicular orthogonal',
  '∥': 'parallel',
  '∠': 'angle',
  '°': 'degree temperature',
  '∴': 'therefore',
  '∵': 'because since',
  '←': 'left arrow',
  '→': 'right arrow',
  '↑': 'up arrow',
  '↓': 'down arrow',
  '↔': 'left right arrow',
  '↕': 'up down arrow',
  '↖': 'northwest arrow up left',
  '↗': 'northeast arrow up right',
  '↘': 'southeast arrow down right',
  '↙': 'southwest arrow down left',
  '⇐': 'left double arrow',
  '⇒': 'right double arrow',
  '⇑': 'up double arrow',
  '⇓': 'down double arrow',
  '⇔': 'left right double arrow',
  '—': 'em dash long dash',
  '–': 'en dash',
  '…': 'ellipsis dots',
  '•': 'bullet dot',
  '«': 'left guillemet',
  '»': 'right guillemet',
  '‹': 'single left angle quote',
  '›': 'single right angle quote',
  '©': 'copyright',
  '®': 'registered trademark',
  '™': 'trademark',
  '§': 'section',
  '¶': 'pilcrow paragraph',
  '†': 'dagger obelus',
  '‡': 'double dagger',
  '‰': 'per mille per thousand',
  '°': 'degree',
  'α': 'alpha greek',
  'β': 'beta greek',
  'γ': 'gamma greek',
  'δ': 'delta greek',
  'ε': 'epsilon greek',
  'θ': 'theta greek',
  'λ': 'lambda greek',
  'μ': 'mu micro',
  'π': 'pi greek',
  'σ': 'sigma greek',
  'τ': 'tau greek',
  'φ': 'phi greek',
  'ω': 'omega greek',
  'Σ': 'sigma uppercase sum',
  'Ω': 'omega uppercase ohm',
  'Δ': 'delta uppercase change',
  '¼': 'one quarter fraction',
  '½': 'one half fraction',
  '¾': 'three quarters fraction',
  '⅓': 'one third fraction',
  '⅔': 'two thirds fraction',
  '─': 'horizontal line box',
  '│': 'vertical line box',
  '┌': 'box top left corner',
  '┐': 'box top right corner',
  '└': 'box bottom left corner',
  '┘': 'box bottom right corner',
  '├': 'box left tee',
  '┤': 'box right tee',
  '┬': 'box top tee',
  '┴': 'box bottom tee',
  '┼': 'box cross',
  '═': 'double horizontal box',
  '║': 'double vertical box',
  '╔': 'double box top left',
  '╗': 'double box top right',
  '╚': 'double box bottom left',
  '╝': 'double box bottom right',
  '╠': 'double box left tee',
  '╣': 'double box right tee',
  '╦': 'double box top tee',
  '╩': 'double box bottom tee',
  '╬': 'double box cross',
}

function getCharName(char: string): string {
  return CHAR_NAMES[char] || ''
}

interface Props {
  onSelect: (char: string) => void
  onClose: () => void
}

export default function SpecialCharacters({ onSelect, onClose }: Props) {
  const [category, setCategory] = useState<keyof typeof SPECIAL_CHAR_CATEGORIES>('symbols')
  const [search, setSearch] = useState('')

  const filteredChars = search
    ? Object.values(SPECIAL_CHAR_CATEGORIES).flat().filter((c) => {
        const name = getCharName(c)
        return name.toLowerCase().includes(search.toLowerCase())
      })
    : SPECIAL_CHAR_CATEGORIES[category]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={onClose}>
      <div className="bg-white rounded-lg shadow-xl w-[500px] max-h-[600px] flex flex-col" onClick={(e) => e.stopPropagation()}>
        {/* Header */}
        <div className="p-3 border-b">
          <div className="flex items-center justify-between mb-2">
            <h3 className="font-semibold text-sm">Insert Special Character</h3>
            <button type="button" onClick={onClose} className="text-gray-500 hover:text-gray-700 text-xl leading-none">×</button>
          </div>
          <input
            type="text"
            placeholder="Search by character or name..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full px-2 py-1 text-sm border rounded"
          />
        </div>

        {/* Categories */}
        {!search && (
          <div className="flex gap-1 px-3 py-2 border-b overflow-x-auto">
            {(Object.keys(SPECIAL_CHAR_CATEGORIES) as Array<keyof typeof SPECIAL_CHAR_CATEGORIES>).map((cat) => (
              <button
                key={cat}
                type="button"
                onClick={() => setCategory(cat)}
                className={`px-2 py-1 text-xs rounded capitalize whitespace-nowrap ${
                  category === cat ? 'bg-blue-100 text-blue-700' : 'hover:bg-gray-100'
                }`}
              >
                {cat}
              </button>
            ))}
          </div>
        )}

        {/* Character Grid */}
        <div className="flex-1 overflow-y-auto p-3">
          <div className="grid grid-cols-12 gap-1">
            {filteredChars.map((char, i) => (
              <button
                key={i}
                type="button"
                onClick={() => {
                  onSelect(char)
                  onClose()
                }}
                className="text-xl hover:bg-blue-50 hover:border-blue-300 border border-transparent rounded p-2 transition-colors flex items-center justify-center"
                title={`U+${char.charCodeAt(0).toString(16).toUpperCase().padStart(4, '0')}`}
              >
                {char}
              </button>
            ))}
          </div>
          {filteredChars.length === 0 && (
            <div className="text-center text-gray-500 text-sm py-8">No characters found</div>
          )}
        </div>

        {/* Footer hint */}
        <div className="px-3 py-2 border-t text-xs text-gray-500">
          Click a character to insert it at cursor position
        </div>
      </div>
    </div>
  )
}
