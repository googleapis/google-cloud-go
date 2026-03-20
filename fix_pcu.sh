sed -i 's/					default:/				default:/g' storage/pcu.go
sed -i 's/						if s.buffersAlloc < s.settings.bufferPoolSize {/					if s.buffersAlloc < s.settings.bufferPoolSize {/g' storage/pcu.go
sed -i 's/							s.currentBuffer = make(\[\]byte, s.config.PartSize)/						s.currentBuffer = make([]byte, s.config.PartSize)/g' storage/pcu.go
sed -i 's/							s.buffersAlloc++/						s.buffersAlloc++/g' storage/pcu.go
sed -i 's/							s.bytesBuffered = 0/						s.bytesBuffered = 0/g' storage/pcu.go
sed -i 's/						} else {/					} else {/g' storage/pcu.go
sed -i 's/							select {/						select {/g' storage/pcu.go
sed -i 's/							case <-s.ctx.Done():/						case <-s.ctx.Done():/g' storage/pcu.go
sed -i 's/								return total - len(p), s.ctx.Err()/							return total - len(p), s.ctx.Err()/g' storage/pcu.go
sed -i 's/							case s.currentBuffer = <-s.bufferCh:/						case s.currentBuffer = <-s.bufferCh:/g' storage/pcu.go
sed -i 's/							s.bytesBuffered = 0/							s.bytesBuffered = 0/g' storage/pcu.go
sed -i 's/						}/					}/g' storage/pcu.go
