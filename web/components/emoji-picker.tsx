// E.113: Emoji picker untuk insert emoji ke dokumen
'use client'

import { useState } from 'react'

const EMOJI_CATEGORIES = {
  smileys: [
    '😀', '😃', '😄', '😁', '😆', '😅', '🤣', '😂', '🙂', '🙃',
    '😉', '😊', '😇', '🥰', '😍', '🤩', '😘', '😗', '😚', '😙',
    '😋', '😛', '😜', '🤪', '😝', '🤑', '🤗', '🤭', '🤫', '🤔',
    '🤐', '🤨', '😐', '😑', '😶', '😏', '😒', '🙄', '😬', '🤥',
    '😌', '😔', '😪', '🤤', '😴', '😷', '🤒', '🤕', '🤢', '🤮',
    '🤧', '🥵', '🥶', '😎', '🤓', '🧐', '😕', '😟', '🙁', '☹️',
    '😮', '😯', '😲', '😳', '🥺', '😦', '�', '😨', '😰', '😥',
    '😢', '😭', '😱', '😖', '😣', '😞', '😓', '😩', '😫', '🥱',
    '😤', '😡', '😠', '🤬', '😈', '👿', '💀', '☠️', '💩', '🤡',
    '👹', '👺', '👻', '👽', '👾', '🤖',
  ],
  gestures: [
    '👋', '🤚', '🖐️', '✋', '🖖', '👌', '🤏', '✌️', '🤞', '🤟',
    '🤘', '🤙', '👈', '👉', '👆', '🖕', '👇', '☝️', '👍', '👎',
    '✊', '👊', '🤛', '🤜', '👏', '🙌', '👐', '🤲', '🤝', '🙏',
    '✍️', '💅', '🤳', '💪', '🦾', '🦿', '🦵', '🦶',
  ],
  hearts: [
    '❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍', '🤎', '💔',
    '❣️', '💕', '💞', '💓', '💗', '💖', '💘', '💝', '💟',
  ],
  objects: [
    '📱', '💻', '⌨️', '🖥️', '🖨️', '🖱️', '💿', '💾', '📀', '📹',
    '📷', '📸', '📞', '☎️', '📟', '📠', '📺', '📻', '🎙️', '🎚️',
    '🎛️', '⏰', '⏱️', '⏲️', '⏳', '⌛', '📡', '🔋', '🔌', '💡',
    '🔦', '🕯️', '🗑️', '🛒', '💰', '💴', '💵', '💶', '💷', '💸',
    '💳', '✉️', '📧', '📨', '📩', '📤', '📥', '📦', '📫', '📪',
    '📬', '📭', '📮', '🗳️', '✏️', '✒️', '🖊️', '🖋️', '🖌️', '🖍️',
    '📝', '📁', '📂', '🗂️', '📅', '📆', '🗒️', '🗓️', '📇', '📈',
    '📉', '📊', '📋', '📌', '📍', '📎', '🔗', '📏', '📐', '✂️',
    '🗃️', '🗄️',
  ],
  symbols: [
    '✅', '✔️', '☑️', '❌', '❎', '➕', '➖', '✖️', '➗', '♾️',
    '💯', '🔢', '🔣', '🆕', '🆓', '🆙', '🆒', '🆗', '🆘', '⚠️',
    '🚫', '🚷', '🚯', '🚱', '🚳', '🚭', '🔞', '📵', '🚸', '⛔',
    '🛑', '❗', '❓', '❕', '❔', '‼️', '⁉️', '💢', '🔆', '🔅',
    '🔰', '⚜️', '💠', '♻️', '⚙️', '🔱', '📛', '⭐', '🌟', '✨',
    '⚡', '☄️', '💥', '🔥', '🌈', '☀️', '🌙',
  ],
  flags: [
    '🏁', '🚩', '🎌', '🏴', '🏳️', '🇮🇩', '🇺🇸', '🇬🇧', '🇫🇷', '🇩🇪',
    '🇯🇵', '🇨🇳', '🇰🇷', '🇮🇳', '🇦🇺', '🇨🇦',
  ],
}

const EMOJI_NAMES: Record<string, string> = {
  '😀': 'grinning face happy smile',
  '😃': 'grinning face with eyes smile',
  '😄': 'grinning face with eyes laugh',
  '😁': 'grin beaming',
  '😆': 'grinning squinting laugh',
  '😅': 'grinning sweat smile',
  '🤣': 'rolling on floor laughing rofl',
  '😂': 'face with tears of joy',
  '🙂': 'slightly smiling',
  '🙃': 'upside down',
  '😉': 'winking',
  '😊': 'smiling with smiling eyes blush',
  '😇': 'smiling halo innocent',
  '🥰': 'smiling hearts love adoration',
  '😍': 'heart eyes love crush',
  '🤩': 'star struck excited wow',
  '😘': 'face blowing a kiss love',
  '😗': 'kissing',
  '😚': 'kissing closed eyes',
  '😙': 'kissing smiling',
  '😋': 'face savoring food yum delicious',
  '😛': 'tongue out playful',
  '😜': 'winking tongue playful',
  '🤪': 'zany crazy wild',
  '😝': 'squinting tongue',
  '🤑': 'money mouth rich money',
  '🤗': 'hugging hug warm',
  '🤭': 'hand over mouth giggle oops',
  '🤫': 'shushing quiet secret',
  '🤔': 'thinking hmmm think',
  '🤐': 'zipper mouth silent quiet',
  '🤨': 'raised eyebrow skeptical',
  '😐': 'neutral face blank',
  '😑': 'expressionless blank meh',
  '😶': 'no mouth silent mute',
  '😏': 'smirk sly suggest',
  '😒': 'unamused annoyed blah',
  '🙄': 'eye roll annoyed',
  '😬': 'grimacing awkward',
  '🤥': 'lying pinocchio',
  '😌': 'relieved content calm',
  '😔': 'pensive sad dejected',
  '😪': 'sleepy tired',
  '🤤': 'drooling hungry',
  '😴': 'sleeping zzz tired',
  '😷': 'medical mask sick face',
  '🤒': 'thermometer sick fever',
  '🤕': 'head bandage hurt',
  '🤢': 'nauseated sick green',
  '🤮': 'vomiting sick throw up',
  '🤧': 'sneezing cold',
  '🥵': 'hot face sweating',
  '🥶': 'cold face freezing',
  '😎': 'sunglasses cool chill',
  '🤓': 'nerd geek smart',
  '🧐': 'monocle thinking inspect',
  '😕': 'confused puzzled',
  '😟': 'worried anxious',
  '🙁': 'slightly frowning sad',
  '☹️': 'frowning sad',
  '😮': 'face with mouth open surprised',
  '😯': 'hushed stunned',
  '😲': 'astonished shocked amazed',
  '😳': 'flushed embarrassed blush',
  '🥺': 'pleading please puppy eyes',
  '😦': 'frowning mouth open',
  '😧': 'anguished distressed',
  '😨': 'fearful scared',
  '😰': 'anxious with sweat nervous',
  '😥': 'sad but relieved',
  '😢': 'crying tear sad',
  '😭': 'loudly crying sobbing',
  '😱': 'screaming in fear horror',
  '😖': 'confounded frustrated',
  '😣': 'persevere struggling',
  '😞': 'disappointed sad down',
  '😓': 'downcast sweat cold',
  '😩': 'weary exhausted',
  '😫': 'tired exhausted',
  '🥱': 'yawning bored tired',
  '😤': 'triumph angry steam',
  '😡': 'pouting anger rage red',
  '😠': 'angry mad',
  '🤬': 'swearing cursing angry symbols',
  '😈': 'smiling horns devil',
  '👿': 'angry horns devil',
  '💀': 'skull dead',
  '☠️': 'skull and crossbones danger',
  '💩': 'poop poo crap',
  '🤡': 'clown face',
  '👹': 'ogre monster',
  '👺': 'goblin tengu',
  '👻': 'ghost halloween spooky',
  '👽': 'alien ufo space',
  '👾': 'alien invader space',
  '🤖': 'robot face bot',
  '👋': 'waving hand hello goodbye wave hi',
  '🤚': 'raised back of hand',
  '🖐️': 'hand with fingers splayed',
  '✋': 'raised hand stop high five',
  '🖖': 'vulcan salute star trek',
  '👌': 'ok hand perfect good',
  '🤏': 'pinching hand small tiny',
  '✌️': 'victory hand peace',
  '🤞': 'crossed fingers good luck hope',
  '🤟': 'love you gesture i love you',
  '🤘': 'rock on horns',
  '🤙': 'call me hand shaka',
  '👈': 'backhand index pointing left',
  '👉': 'backhand index pointing right',
  '👆': 'backhand index pointing up',
  '🖕': 'middle finger flip off',
  '👇': 'backhand index pointing down',
  '☝️': 'index pointing up point',
  '👍': 'thumbs up like approve yes good',
  '👎': 'thumbs down dislike no bad',
  '✊': 'raised fist solidarity power',
  '👊': 'oncoming fist punch bump',
  '🤛': 'left facing fist',
  '🤜': 'right facing fist',
  '👏': 'clapping hands applause bravo',
  '🙌': 'raising hands celebration hooray',
  '👐': 'open hands',
  '🤲': 'palms up together',
  '🤝': 'handshake deal agree',
  '🙏': 'folded hands please thank you pray',
  '✍️': 'writing hand',
  '💅': 'nail polish beauty',
  '🤳': 'selfie',
  '💪': 'flexed biceps strong muscle',
  '❤️': 'red heart love',
  '🧡': 'orange heart',
  '💛': 'yellow heart',
  '💚': 'green heart',
  '💙': 'blue heart',
  '💜': 'purple heart',
  '🖤': 'black heart dark',
  '🤍': 'white heart',
  '🤎': 'brown heart',
  '💔': 'broken heart sad',
  '❣️': 'heart exclamation',
  '💕': 'two hearts love',
  '💞': 'revolving hearts',
  '💓': 'beating heart',
  '💗': 'growing heart',
  '💖': 'sparkling heart',
  '💘': 'heart with arrow cupid',
  '💝': 'heart with ribbon gift',
  '💟': 'heart decoration',
  '✅': 'check mark done complete green',
  '❌': 'cross mark wrong no',
  '➕': 'plus sign add',
  '➖': 'minus sign subtract',
  '✖️': 'multiply sign',
  '💯': 'hundred points perfect score',
  '⚠️': 'warning caution alert',
  '🚫': 'prohibited no not allowed',
  '⛔': 'no entry stop',
  '🛑': 'stop sign red octagonal',
  '❗': 'exclamation red mark',
  '❓': 'question mark red',
  '💢': 'anger symbol',
  '🔥': 'fire hot flame lit burn',
  '⭐': 'star',
  '🌟': 'glowing star bright',
  '✨': 'sparkles stars magic',
  '⚡': 'lightning bolt zap thunder',
  '🌈': 'rainbow color',
  '☀️': 'sun light bright sunny',
  '🌙': 'crescent moon night',
  '🏆': 'trophy winner prize',
  '🎉': 'party popper celebrate',
  '🎊': 'confetti ball',
  '💐': 'bouquet flowers',
  '🎈': 'balloon',
  '📌': 'pushpin pin',
  '📎': 'paperclip attach',
  '📝': 'memo note write',
  '📁': 'file folder',
  '📂': 'open file folder',
  '🗑️': 'wastebasket trash delete',
  '💰': 'money bag',
  '💳': 'credit card payment',
  '🔗': 'link chain',
  '📧': 'email envelope',
  '📱': 'mobile phone smartphone',
  '💻': 'laptop computer',
  '🖥️': 'desktop computer monitor',
  '⌨️': 'keyboard',
  '📷': 'camera photo',
  '📸': 'camera with flash',
  '🎙️': 'studio microphone',
  '⏰': 'alarm clock',
  '⏱️': 'stopwatch',
  '⏳': 'hourglass',
  '📅': 'calendar',
  '📊': 'bar chart',
  '📈': 'chart increasing up',
  '📉': 'chart decreasing down',
  '📋': 'clipboard',
  '🔒': 'locked secure',
  '🔓': 'unlocked open',
  '🔑': 'key',
  '🏁': 'chequered flag',
  '🚩': 'red flag',
  '🏴': 'black flag',
  '🏳️': 'white flag',
  '✅': 'check mark done',
  '㊗️': 'congratulations',
  '㊙️': 'secret',
}

interface Props {
  onSelect: (emoji: string) => void
  onClose: () => void
}

export default function EmojiPicker({ onSelect, onClose }: Props) {
  const [category, setCategory] = useState<keyof typeof EMOJI_CATEGORIES>('smileys')
  const [search, setSearch] = useState('')

  const filteredEmojis = search
    ? Object.values(EMOJI_CATEGORIES).flat().filter((e) => {
        const name = EMOJI_NAMES[e] || ''
        return name.toLowerCase().includes(search.toLowerCase())
      })
    : EMOJI_CATEGORIES[category]

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30" onClick={onClose}>
      <div className="bg-white rounded-lg shadow-xl w-[400px] max-h-[500px] flex flex-col" onClick={(e) => e.stopPropagation()}>
        {/* Header */}
        <div className="p-3 border-b">
          <div className="flex items-center justify-between mb-2">
            <h3 className="font-semibold text-sm">Insert Emoji</h3>
            <button type="button" onClick={onClose} className="text-gray-500 hover:text-gray-700 text-xl leading-none">×</button>
          </div>
          <input
            type="text"
            placeholder="Search emoji..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full px-2 py-1 text-sm border rounded"
          />
        </div>

        {/* Categories */}
        {!search && (
          <div className="flex gap-1 px-3 py-2 border-b overflow-x-auto">
            {(Object.keys(EMOJI_CATEGORIES) as Array<keyof typeof EMOJI_CATEGORIES>).map((cat) => (
              <button
                key={cat}
                type="button"
                onClick={() => setCategory(cat)}
                className={`px-2 py-1 text-xs rounded capitalize ${
                  category === cat ? 'bg-blue-100 text-blue-700' : 'hover:bg-gray-100'
                }`}
              >
                {cat}
              </button>
            ))}
          </div>
        )}

        {/* Emoji Grid */}
        <div className="flex-1 overflow-y-auto p-3">
          <div className="grid grid-cols-8 gap-1">
            {filteredEmojis.map((emoji, i) => (
              <button
                key={i}
                type="button"
                onClick={() => {
                  onSelect(emoji)
                  onClose()
                }}
                className="text-2xl hover:bg-gray-100 rounded p-1 transition-colors"
                title={emoji}
              >
                {emoji}
              </button>
            ))}
          </div>
          {filteredEmojis.length === 0 && (
            <div className="text-center text-gray-500 text-sm py-8">No emoji found</div>
          )}
        </div>
      </div>
    </div>
  )
}
