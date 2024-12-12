'use client';

import { FormEvent, useState } from 'react';

export default function SubmitForm() {
    const [result, setResult] = useState(null)
    async function onSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault()
     
        const formData = new FormData(event.currentTarget)
        const response = await fetch(process.env.NEXT_PUBLIC_API_URL + '/api/uploadImage', {
          method: 'POST',
          body: formData,
        })
     
        // Handle response if necessary
        const data = await response.json()
        console.log(data)
        setResult(data.message)
        // ...
    }
     
    return (
      <div>
        <form onSubmit={onSubmit}>
          <input type="file" name="image" accept="image/jpeg" />
          <button type="submit">Submit</button>
        </form>
        <div>
          <p>Result: {result}</p>
        </div>
      </div>
    )
}