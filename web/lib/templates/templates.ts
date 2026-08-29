// Q.288-299: Template Gallery (Resume, Report, Proposal, etc.)
// Fitur wajib Google Docs - template untuk berbagai use case

export interface DocumentTemplate {
  id: string
  name: string
  description: string
  category: 'personal' | 'work' | 'education' | 'creative'
  icon: string
  content: string // ProseMirror JSON atau Markdown
}

// Q.288: Template Resume/CV
export const TEMPLATE_RESUME: DocumentTemplate = {
  id: 'resume-cv',
  name: 'Resume / CV',
  description: 'Professional resume template with sections for experience, education, and skills',
  category: 'personal',
  icon: '📄',
  content: `# [Your Name]

**[Your Title/Position]**  
📧 email@example.com | 📱 +62 xxx-xxxx | 🔗 linkedin.com/in/yourname

---

## Professional Summary

[Brief professional summary highlighting key achievements and skills - 2-3 sentences]

---

## Work Experience

### [Job Title] | [Company Name]
*[Start Date] - [End Date | Present]*

- Achievement or responsibility 1
- Achievement or responsibility 2
- Achievement or responsibility 3

### [Previous Job Title] | [Previous Company]
*[Start Date] - [End Date]*

- Achievement or responsibility 1
- Achievement or responsibility 2

---

## Education

### [Degree Name] | [University Name]
*[Graduation Year]*

- GPA: [X.XX/4.00] (if relevant)
- Relevant coursework: [List key courses]

---

## Skills

**Technical Skills:** [Skill 1] • [Skill 2] • [Skill 3]  
**Languages:** [Language 1] (Native) • [Language 2] (Fluent)  
**Tools & Technologies:** [Tool 1] • [Tool 2] • [Tool 3]

---

## Certifications & Awards

- [Certification Name] - [Issuing Organization], [Year]
- [Award Name] - [Description], [Year]
`,
}

// Q.289: Template Cover Letter / Surat Lamaran
export const TEMPLATE_COVER_LETTER: DocumentTemplate = {
  id: 'cover-letter',
  name: 'Cover Letter',
  description: 'Professional cover letter template for job applications',
  category: 'personal',
  icon: '✉️',
  content: `[Your Name]  
[Your Address]  
[City, Postal Code]  
[Your Email]  
[Your Phone]

[Date]

[Hiring Manager's Name]  
[Title]  
[Company Name]  
[Company Address]  
[City, Postal Code]

Dear [Hiring Manager's Name],

I am writing to express my strong interest in the [Position Title] position at [Company Name], as advertised on [where you found the job posting]. With [X years] of experience in [your field/industry] and a proven track record of [key achievement or skill], I am confident that I would be a valuable addition to your team.

In my current role as [Your Current Position] at [Current Company], I have successfully [describe 2-3 key achievements or responsibilities that relate to the job you're applying for]. These experiences have equipped me with [relevant skills] that directly align with the requirements outlined in your job description.

I am particularly drawn to [Company Name] because of [specific reason related to company's mission, values, or recent projects]. I am excited about the opportunity to contribute to [specific company goal or project] and believe my background in [relevant area] would enable me to make meaningful contributions from day one.

Thank you for considering my application. I would welcome the opportunity to discuss how my skills and experiences align with [Company Name]'s needs. I am available for an interview at your convenience and can be reached at [your phone] or [your email].

Sincerely,

[Your Name]
`,
}

// Q.290: Template Report / Laporan
export const TEMPLATE_REPORT: DocumentTemplate = {
  id: 'report-business',
  name: 'Business Report',
  description: 'Structured template for business reports and analysis documents',
  category: 'work',
  icon: '📊',
  content: `# [Report Title]

**Prepared by:** [Your Name]  
**Date:** [Date]  
**Department:** [Department Name]  
**Version:** 1.0

---

## Executive Summary

[Provide a brief overview of the report - key findings, conclusions, and recommendations. This should be 3-5 paragraphs that someone can read quickly to understand the core message.]

---

## Table of Contents

1. Introduction
2. Objectives
3. Methodology
4. Findings & Analysis
5. Conclusions
6. Recommendations
7. Appendices

---

## 1. Introduction

### Background

[Provide context for why this report was created. What problem or opportunity does it address?]

### Scope

[Define what is included and excluded from this report.]

---

## 2. Objectives

The primary objectives of this report are to:

- [Objective 1]
- [Objective 2]
- [Objective 3]

---

## 3. Methodology

[Describe how data was collected and analyzed. Include:]

- **Data Sources:** [List sources]
- **Time Period:** [Specify timeframe]
- **Analysis Methods:** [Describe approach]

---

## 4. Findings & Analysis

### Key Finding 1: [Title]

[Detailed explanation of the finding with supporting data]

### Key Finding 2: [Title]

[Detailed explanation with data]

### Key Finding 3: [Title]

[Detailed explanation with data]

---

## 5. Conclusions

[Summarize the main conclusions drawn from the findings. What do the findings mean for the organization?]

---

## 6. Recommendations

Based on the findings and conclusions, we recommend:

1. **[Recommendation 1]**  
   - Action steps
   - Timeline
   - Resources required

2. **[Recommendation 2]**  
   - Action steps
   - Timeline
   - Resources required

---

## 7. Appendices

### Appendix A: Supporting Data

[Include tables, charts, or additional data]

### Appendix B: References

[List sources and references]
`,
}

// Q.292: Template Project Proposal
export const TEMPLATE_PROPOSAL: DocumentTemplate = {
  id: 'project-proposal',
  name: 'Project Proposal',
  description: 'Complete project proposal template with budget and timeline',
  category: 'work',
  icon: '📋',
  content: `# Project Proposal: [Project Name]

**Submitted by:** [Your Name/Team]  
**Date:** [Date]  
**Project Duration:** [Estimated Duration]  
**Budget Request:** [Amount]

---

## Executive Summary

[2-3 paragraph summary of the entire proposal - what you want to do, why it matters, and what you need]

---

## Project Background

### Problem Statement

[Clearly define the problem or opportunity this project addresses]

### Current Situation

[Describe the current state and why change is needed]

---

## Project Objectives

The key objectives of this project are:

1. [Objective 1 - must be SMART: Specific, Measurable, Achievable, Relevant, Time-bound]
2. [Objective 2]
3. [Objective 3]

---

## Scope

### In Scope

- [What will be included]
- [Deliverable 1]
- [Deliverable 2]

### Out of Scope

- [What will NOT be included]
- [Exclusion 1]

---

## Methodology & Approach

[Describe how the project will be executed]

### Phase 1: [Phase Name] (Weeks 1-4)

- Task 1
- Task 2
- Deliverable

### Phase 2: [Phase Name] (Weeks 5-8)

- Task 1
- Task 2
- Deliverable

---

## Timeline

| Milestone | Target Date | Status |
|-----------|-------------|--------|
| Project Kickoff | [Date] | Planned |
| Phase 1 Complete | [Date] | Planned |
| Phase 2 Complete | [Date] | Planned |
| Final Delivery | [Date] | Planned |

---

## Budget

| Category | Cost (USD) | Notes |
|----------|------------|-------|
| Personnel | $X,XXX | [Details] |
| Equipment | $X,XXX | [Details] |
| Software/Licenses | $X,XXX | [Details] |
| Miscellaneous | $X,XXX | [Details] |
| **TOTAL** | **$XX,XXX** | |

---

## Team & Resources

### Project Team

- **Project Manager:** [Name]
- **Lead Developer:** [Name]
- **Designer:** [Name]

### Required Resources

- [Resource 1]
- [Resource 2]

---

## Risk Management

| Risk | Probability | Impact | Mitigation Strategy |
|------|------------|--------|---------------------|
| [Risk 1] | Medium | High | [How to mitigate] |
| [Risk 2] | Low | Medium | [How to mitigate] |

---

## Success Metrics

How will we measure success?

- [Metric 1: e.g., 20% increase in user engagement]
- [Metric 2: e.g., Delivered on time and within budget]
- [Metric 3: e.g., Positive stakeholder feedback]

---

## Conclusion

[Summarize why this project should be approved and what the expected benefits are]

---

## Approval

**Prepared by:**  
[Your Name], [Title]  
Signature: _________________ Date: _______

**Approved by:**  
[Approver Name], [Title]  
Signature: _________________ Date: _______
`,
}

// Q.293: Template Business Plan
export const TEMPLATE_BUSINESS_PLAN: DocumentTemplate = {
  id: 'business-plan',
  name: 'Business Plan',
  description: 'Comprehensive business plan template for startups and entrepreneurs',
  category: 'work',
  icon: '💼',
  content: `# Business Plan: [Company Name]

**Prepared by:** [Founder Name(s)]  
**Date:** [Date]  
**Version:** 1.0  
**Confidential**

---

## Executive Summary

[1-2 page overview of your entire business plan. Write this LAST after completing all other sections.]

- **Business Concept:** [What you do]
- **Target Market:** [Who you serve]
- **Competitive Advantage:** [Why you'll win]
- **Financial Highlights:** [Key numbers]
- **Funding Request:** [If applicable]

---

## Company Description

### Mission Statement

[Your company's purpose and core values in 1-2 sentences]

### Vision

[Where you see the company in 5-10 years]

### Company History

[How the company was founded, key milestones]

### Legal Structure

[Corporation, LLC, Partnership, etc.]

---

## Products & Services

### Product/Service 1: [Name]

**Description:** [What it is and what problem it solves]  
**Features:** [Key features]  
**Benefits:** [Value to customer]  
**Pricing:** [Pricing model]

### Product/Service 2: [Name]

[Same structure as above]

### Future Offerings

[Products/services planned for future development]

---

## Market Analysis

### Industry Overview

[Size, growth trends, key characteristics of your industry]

### Target Market

**Primary Target:** [Demographics, psychographics, behaviors]  
**Market Size:** [TAM, SAM, SOM estimates]  
**Customer Personas:** [Detailed descriptions of ideal customers]

### Market Trends

- [Trend 1 and its impact on your business]
- [Trend 2]
- [Trend 3]

---

## Competitive Analysis

### Direct Competitors

| Competitor | Strengths | Weaknesses | Market Share |
|------------|-----------|------------|--------------|
| [Company 1] | [List] | [List] | X% |
| [Company 2] | [List] | [List] | X% |

### Competitive Advantage

[What makes your business unique and defensible?]

---

## Marketing & Sales Strategy

### Marketing Strategy

**Positioning:** [How you want to be perceived]  
**Channels:** [Where you'll reach customers]  
**Budget:** [Marketing spend allocation]

### Sales Strategy

**Sales Process:** [How you convert prospects to customers]  
**Sales Team:** [Structure and compensation]  
**Sales Forecast:** [Year 1-3 projections]

---

## Operations Plan

### Location

[Physical location, facilities, reasons for location]

### Technology & Equipment

[Key technology, tools, equipment needed]

### Suppliers & Partners

[Key relationships and why they matter]

### Production/Service Delivery

[How products/services are created and delivered]

---

## Management Team

### [Founder/CEO Name]

**Role:** [Title]  
**Background:** [Relevant experience]  
**Responsibilities:** [Key duties]

### [Co-founder/CTO Name]

[Same structure]

### Advisory Board

- [Advisor 1] - [Expertise]
- [Advisor 2] - [Expertise]

---

## Financial Projections

### Revenue Model

[How you make money - pricing, volume assumptions]

### 3-Year Financial Summary

|  | Year 1 | Year 2 | Year 3 |
|--|--------|--------|--------|
| Revenue | $XXX,XXX | $XXX,XXX | $X,XXX,XXX |
| COGS | $XX,XXX | $XX,XXX | $XXX,XXX |
| Gross Profit | $XXX,XXX | $XXX,XXX | $XXX,XXX |
| Operating Expenses | $XXX,XXX | $XXX,XXX | $XXX,XXX |
| Net Income | $(XX,XXX) | $XX,XXX | $XXX,XXX |

### Break-Even Analysis

[When and how you'll reach profitability]

### Use of Funds

[If seeking investment, detail how funds will be used]

---

## Risk Analysis

### Key Risks

1. **[Risk Category]:** [Description and mitigation]
2. **[Risk Category]:** [Description and mitigation]
3. **[Risk Category]:** [Description and mitigation]

---

## Appendices

### Appendix A: Detailed Financial Statements

[Income statement, cash flow, balance sheet]

### Appendix B: Market Research Data

[Supporting data and sources]

### Appendix C: Product Details

[Technical specs, mockups, etc.]
`,
}

// Q.295: Template Meeting Notes
export const TEMPLATE_MEETING_NOTES: DocumentTemplate = {
  id: 'meeting-notes',
  name: 'Meeting Notes',
  description: 'Structured template for capturing meeting discussions and action items',
  category: 'work',
  icon: '📝',
  content: `# Meeting Notes: [Meeting Title]

**Date:** [Date]  
**Time:** [Start Time] - [End Time]  
**Location:** [Room/Virtual Link]  
**Facilitator:** [Name]  
**Note Taker:** [Name]

---

## Attendees

**Present:**
- [Name 1] - [Role]
- [Name 2] - [Role]
- [Name 3] - [Role]

**Absent:**
- [Name] - [Role]

---

## Agenda

1. [Agenda Item 1]
2. [Agenda Item 2]
3. [Agenda Item 3]
4. [Agenda Item 4]
5. AOB (Any Other Business)

---

## Discussion Notes

### 1. [Agenda Item 1]

**Presenter:** [Name]

**Key Points:**
- [Point 1]
- [Point 2]
- [Point 3]

**Questions/Concerns:**
- Q: [Question] - A: [Answer]

**Decisions Made:**
- ✅ [Decision 1]
- ✅ [Decision 2]

---

### 2. [Agenda Item 2]

[Same structure as above]

---

## Action Items

| # | Action | Owner | Due Date | Status |
|---|--------|-------|----------|--------|
| 1 | [Action description] | [Name] | [Date] | 🟡 In Progress |
| 2 | [Action description] | [Name] | [Date] | ⚪ Not Started |
| 3 | [Action description] | [Name] | [Date] | ✅ Complete |

---

## Parking Lot

*Items for future discussion:*
- [Topic that came up but wasn't addressed]
- [Another deferred topic]

---

## Next Meeting

**Date:** [Date]  
**Time:** [Time]  
**Agenda Preview:**
- Review action items
- [Next topic]
- [Next topic]
`,
}

// Q.294: Template Newsletter
export const TEMPLATE_NEWSLETTER: DocumentTemplate = {
  id: 'newsletter',
  name: 'Newsletter',
  description: 'Email newsletter template for company updates and announcements',
  category: 'work',
  icon: '📰',
  content: `# [Newsletter Name] - [Month Year] Edition

*[Tagline or brief description]*

---

## 📢 Top Story

### [Headline]

[Lead story with 2-3 paragraphs. This should be your most important or exciting news.]

**[Read more →](#)**

---

## 🎯 What's New

### [Update 1 Title]

[Brief description - 1-2 paragraphs]

### [Update 2 Title]

[Brief description]

### [Update 3 Title]

[Brief description]

---

## 💡 Feature Spotlight

**[Feature or Product Name]**

[Detailed look at a feature, product, or service. Include benefits and how to use it.]

- **Benefit 1:** [Description]
- **Benefit 2:** [Description]
- **Benefit 3:** [Description]

**[Try it now →](#)**

---

## 👥 Team Highlight

### Meet [Team Member Name]

*[Role/Title]*

[Brief bio or interview snippet. Humanize your company by featuring team members.]

---

## 📅 Upcoming Events

- **[Event Name]** - [Date] | [Location/Link]
- **[Event Name]** - [Date] | [Location/Link]
- **[Event Name]** - [Date] | [Location/Link]

---

## 📊 By The Numbers

This month:
- 📈 [Metric]: [Number]
- 🎉 [Achievement]: [Number]
- 👥 [Stat]: [Number]

---

## 🔗 Quick Links

- [Link Title] - [Brief description]
- [Link Title] - [Brief description]
- [Link Title] - [Brief description]

---

## 💬 From Our Community

> "[Customer testimonial or quote from community member]"
> 
> — [Name], [Title/Company]

---

## 📣 Stay Connected

Follow us:
- 🐦 Twitter: [@handle](#)
- 📘 LinkedIn: [Company Page](#)
- 📷 Instagram: [@handle](#)

---

*Questions or feedback? Reply to this email or contact us at [email@company.com]*

**[Unsubscribe](#)** | **[Update Preferences](#)** | **[View Online](#)**
`,
}

// Export all templates
export const ALL_TEMPLATES: DocumentTemplate[] = [
  TEMPLATE_RESUME,
  TEMPLATE_COVER_LETTER,
  TEMPLATE_REPORT,
  TEMPLATE_PROPOSAL,
  TEMPLATE_BUSINESS_PLAN,
  TEMPLATE_MEETING_NOTES,
  TEMPLATE_NEWSLETTER,
]

export function getTemplateById(id: string): DocumentTemplate | undefined {
  return ALL_TEMPLATES.find((t) => t.id === id)
}

export function getTemplatesByCategory(category: DocumentTemplate['category']): DocumentTemplate[] {
  return ALL_TEMPLATES.filter((t) => t.category === category)
}
